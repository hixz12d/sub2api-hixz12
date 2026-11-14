package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ResetCardHandler 重置卡用户接口。功能开关关闭时返回 404（reason=RESET_CARD_DISABLED）。
type ResetCardHandler struct {
	resetCardService *service.ResetCardService
}

// NewResetCardHandler 创建重置卡用户接口处理器。
func NewResetCardHandler(resetCardService *service.ResetCardService) *ResetCardHandler {
	return &ResetCardHandler{resetCardService: resetCardService}
}

// ResetCardPopupSeenRequest 标记已弹窗请求。
type ResetCardPopupSeenRequest struct {
	CardIDs []int64 `json:"card_ids"`
}

// List 列出我的卡
// GET /api/v1/reset-cards
func (h *ResetCardHandler) List(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	result, err := h.resetCardService.ListUserCards(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// Preview 实时重算一张可用卡的预估返还
// GET /api/v1/reset-cards/:id/preview
func (h *ResetCardHandler) Preview(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	cardID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || cardID <= 0 {
		response.BadRequest(c, "Invalid reset card ID")
		return
	}
	result, err := h.resetCardService.PreviewUserCard(c.Request.Context(), subject.UserID, cardID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// Use 使用一张卡
// POST /api/v1/reset-cards/:id/use
func (h *ResetCardHandler) Use(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	cardID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || cardID <= 0 {
		response.BadRequest(c, "Invalid reset card ID")
		return
	}
	result, err := h.resetCardService.UseCard(c.Request.Context(), subject.UserID, cardID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// MarkPopupSeen 把这些卡标记为已弹过窗
// POST /api/v1/reset-cards/popup-seen
func (h *ResetCardHandler) MarkPopupSeen(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req ResetCardPopupSeenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	updated, err := h.resetCardService.MarkPopupSeen(c.Request.Context(), subject.UserID, req.CardIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"updated": updated})
}
