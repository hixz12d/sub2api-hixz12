package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *AccountHandler) SetQuestionReviews(reviews *service.QuestionReviewService) {
	h.questionReviews = reviews
}
func questionActor(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	role, roleOK := middleware.GetUserRoleFromContext(c)
	if !ok || !roleOK || subject.UserID <= 0 || role != service.RoleAdmin {
		response.Forbidden(c, "Administrator required")
		return 0, false
	}
	return subject.UserID, true
}
func questionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrQuestionReviewDenied):
		response.Forbidden(c, "Question review unavailable")
	case errors.Is(err, service.ErrQuestionReviewConflict):
		response.Error(c, http.StatusConflict, "Review changed; reload before submitting")
	case errors.Is(err, sql.ErrNoRows):
		response.NotFound(c, "Question record not found")
	default:
		response.InternalError(c, "Question operation failed")
	}
}
func (h *AccountHandler) beginQuestionCapture(c *gin.Context, accountID int64, req TestAccountRequest) (func(), bool) {
	actor, ok := questionActor(c)
	if !ok {
		return nil, false
	}
	if err := h.questionReviews.Authorize(c.Request.Context(), actor); err != nil {
		questionError(c, err)
		return nil, false
	}
	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil || account == nil {
		response.NotFound(c, "Account unavailable")
		return nil, false
	}
	// Credential fields are kept in memory solely to suppress echoed secrets.
	var secrets []string
	for _, name := range []string{"api_key", "access_token", "refresh_token", "id_token", "client_secret"} {
		if value, ok := account.Credentials[name].(string); ok && value != "" {
			secrets = append(secrets, value)
		}
	}
	capture := service.NewQuestionCapture(accountID, actor, req.ModelID, req.Prompt, secrets)
	c.Set("account_question_capture", capture)
	return func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 5*time.Second)
		defer cancel()
		id, err := h.questionReviews.Save(ctx, actor, capture)
		event := gin.H{"type": "question_record", "record_id": id, "saved": err == nil}
		if err != nil {
			event["record_id"] = ""
		}
		raw, _ := json.Marshal(event)
		_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", raw)
		c.Writer.Flush()
	}, true
}
func (h *AccountHandler) Questions(c *gin.Context) {
	actor, ok := questionActor(c)
	if !ok {
		return
	}
	account, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || account <= 0 {
		response.BadRequest(c, "Invalid account")
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 201 {
		response.BadRequest(c, "Invalid page")
		return
	}
	items, err := h.questionReviews.List(c.Request.Context(), actor, account, 50, (page-1)*50)
	if err != nil {
		questionError(c, err)
		return
	}
	response.Success(c, gin.H{"items": items, "page": page, "has_more": len(items) == 50})
}
func (h *AccountHandler) QuestionHistory(c *gin.Context) {
	actor, ok := questionActor(c)
	if !ok {
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 201 {
		response.BadRequest(c, "Invalid page")
		return
	}
	items, err := h.questionReviews.History(c.Request.Context(), actor, c.Param("record"), 50, (page-1)*50)
	if err != nil {
		questionError(c, err)
		return
	}
	response.Success(c, gin.H{"items": items, "page": page, "has_more": len(items) == 50})
}
func (h *AccountHandler) ReviewQuestion(c *gin.Context) {
	actor, ok := questionActor(c)
	if !ok {
		return
	}
	var input struct {
		Verdict  string `json:"verdict"`
		Reason   string `json:"reason"`
		Revision *int64 `json:"expected_revision"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || input.Revision == nil {
		response.BadRequest(c, "Invalid review")
		return
	}
	result, err := h.questionReviews.Review(c.Request.Context(), actor, c.Param("record"), input.Verdict, input.Reason, *input.Revision)
	if err != nil {
		questionError(c, err)
		return
	}
	response.Success(c, result)
}
