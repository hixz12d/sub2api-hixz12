package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type BenchmarkAdminService interface {
	List(context.Context, int64, int, int) ([]service.BenchmarkReleaseSummary, []service.BenchmarkChannel, error)
	Stage(context.Context, int64, service.BenchmarkPackageInput) (string, error)
	Approve(context.Context, int64, string) error
	Activate(context.Context, int64, string, string, int64) (int64, error)
	Withdraw(context.Context, int64, string, string) error
}

type BenchmarkHandler struct{ service BenchmarkAdminService }

func NewBenchmarkHandler(svc *service.BenchmarkService) *BenchmarkHandler {
	return &BenchmarkHandler{service: svc}
}

func benchmarkActor(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	role, roleOK := middleware.GetUserRoleFromContext(c)
	if !ok || !roleOK || subject.UserID <= 0 || role != service.RoleAdmin {
		response.Forbidden(c, "Benchmark administrator required")
		return 0, false
	}
	return subject.UserID, true
}

func benchmarkError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrBenchmarkAdmin):
		response.Forbidden(c, "Benchmark administrator required")
	case errors.Is(err, service.ErrBenchmarkRelease), errors.Is(err, sql.ErrNoRows):
		c.JSON(http.StatusConflict, gin.H{"code": http.StatusConflict, "message": "Benchmark unavailable or version conflict"})
	case errors.Is(err, service.ErrBenchmarkValidator):
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": http.StatusServiceUnavailable, "message": "Offline validation unavailable or candidate rejected"})
	default:
		response.InternalError(c, "Benchmark operation failed")
	}
}

func benchmarkDecode(c *gin.Context, value any, maximum int64) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maximum)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		response.BadRequest(c, "Invalid benchmark request")
		return false
	}
	return true
}

func (h *BenchmarkHandler) List(c *gin.Context) {
	actor, ok := benchmarkActor(c)
	if !ok {
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 2001 {
		response.BadRequest(c, "Invalid page")
		return
	}
	items, channels, err := h.service.List(c.Request.Context(), actor, 50, (page-1)*50)
	if err != nil {
		benchmarkError(c, err)
		return
	}
	response.Success(c, gin.H{"items": items, "channels": channels, "page": page, "page_size": 50, "has_more": len(items) == 50})
}

func (h *BenchmarkHandler) Stage(c *gin.Context) {
	actor, ok := benchmarkActor(c)
	if !ok {
		return
	}
	var input struct {
		ID            string `json:"benchmark_id"`
		Version       string `json:"version"`
		SHA256        string `json:"sha256"`
		ContentSHA256 string `json:"content_sha256"`
		Mode          string `json:"mode"`
		Payload       []byte `json:"payload"`
	}
	if !benchmarkDecode(c, &input, 45*1024*1024) {
		return
	}
	id, err := h.service.Stage(c.Request.Context(), actor, service.BenchmarkPackageInput{ID: input.ID, Version: input.Version, SHA256: input.SHA256, ContentSHA256: input.ContentSHA256, Mode: input.Mode, Payload: input.Payload})
	if err != nil {
		benchmarkError(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}

func (h *BenchmarkHandler) Approve(c *gin.Context) {
	actor, ok := benchmarkActor(c)
	if !ok {
		return
	}
	var input struct{}
	if !benchmarkDecode(c, &input, 1024) {
		return
	}
	if err := h.service.Approve(c.Request.Context(), actor, c.Param("id")); err != nil {
		benchmarkError(c, err)
		return
	}
	response.Success(c, gin.H{"approved": true})
}

func (h *BenchmarkHandler) Activate(c *gin.Context) {
	actor, ok := benchmarkActor(c)
	if !ok {
		return
	}
	var input struct {
		Channel  string `json:"channel"`
		Revision *int64 `json:"expected_revision"`
	}
	if !benchmarkDecode(c, &input, 4096) {
		return
	}
	if input.Revision == nil {
		response.BadRequest(c, "Expected revision required")
		return
	}
	revision, err := h.service.Activate(c.Request.Context(), actor, input.Channel, c.Param("id"), *input.Revision)
	if err != nil {
		benchmarkError(c, err)
		return
	}
	response.Success(c, gin.H{"revision": revision})
}

func (h *BenchmarkHandler) Withdraw(c *gin.Context) {
	actor, ok := benchmarkActor(c)
	if !ok {
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if !benchmarkDecode(c, &input, 4096) {
		return
	}
	if err := h.service.Withdraw(c.Request.Context(), actor, c.Param("id"), input.Reason); err != nil {
		benchmarkError(c, err)
		return
	}
	response.Success(c, gin.H{"withdrawn": true})
}
