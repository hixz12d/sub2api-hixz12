package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// resetCardMaxPageSize 管理端列表单页上限。
const resetCardMaxPageSize = 100

// ResetCardHandler 重置卡管理员接口（不受功能开关限制）。
type ResetCardHandler struct {
	resetCardService *service.ResetCardService
}

// NewResetCardHandler 创建重置卡管理员接口处理器。
func NewResetCardHandler(resetCardService *service.ResetCardService) *ResetCardHandler {
	return &ResetCardHandler{resetCardService: resetCardService}
}

// ResetCardBatchPreviewRequest 发卡预估请求。
type ResetCardBatchPreviewRequest struct {
	GroupIDs   []int64 `json:"group_ids"`
	Percent    float64 `json:"percent"`
	MaxAmount  float64 `json:"max_amount"`
	TargetMode string  `json:"target_mode"`
	OnlyActive bool    `json:"only_active"`
	UserIDs    []int64 `json:"user_ids"`
}

// CreateResetCardBatchRequest 创建批次请求。
type CreateResetCardBatchRequest struct {
	Name            string  `json:"name"`
	GroupIDs        []int64 `json:"group_ids"`
	Percent         float64 `json:"percent"`
	MaxAmount       float64 `json:"max_amount"`
	CardValidDays   int     `json:"card_valid_days"`
	CreditValidDays int     `json:"credit_valid_days"`
	TargetMode      string  `json:"target_mode"`
	OnlyActive      bool    `json:"only_active"`
	UserIDs         []int64 `json:"user_ids"`
}

func parseResetCardPagination(c *gin.Context) (int, int) {
	page, pageSize := response.ParsePagination(c)
	if pageSize > resetCardMaxPageSize {
		pageSize = resetCardMaxPageSize
	}
	return page, pageSize
}

func parseResetCardID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid ID")
		return 0, false
	}
	return id, true
}

// ListBatches 分页列出批次
// GET /api/v1/admin/reset-cards/batches
func (h *ResetCardHandler) ListBatches(c *gin.Context) {
	page, pageSize := parseResetCardPagination(c)
	items, total, err := h.resetCardService.ListBatches(c.Request.Context(), page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

// PreviewBatch 发卡前预估
// POST /api/v1/admin/reset-cards/batches/preview
func (h *ResetCardHandler) PreviewBatch(c *gin.Context) {
	var req ResetCardBatchPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.resetCardService.PreviewBatch(c.Request.Context(), service.ResetCardBatchInput{
		GroupIDs:   req.GroupIDs,
		Percent:    req.Percent,
		MaxAmount:  req.MaxAmount,
		TargetMode: req.TargetMode,
		OnlyActive: req.OnlyActive,
		UserIDs:    req.UserIDs,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// CreateBatch 创建批次并发卡
// POST /api/v1/admin/reset-cards/batches
func (h *ResetCardHandler) CreateBatch(c *gin.Context) {
	var req CreateResetCardBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	var createdBy int64
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok {
		createdBy = subject.UserID
	}
	result, err := h.resetCardService.CreateBatch(c.Request.Context(), service.ResetCardBatchInput{
		Name:            req.Name,
		GroupIDs:        req.GroupIDs,
		Percent:         req.Percent,
		MaxAmount:       req.MaxAmount,
		CardValidDays:   req.CardValidDays,
		CreditValidDays: req.CreditValidDays,
		TargetMode:      req.TargetMode,
		OnlyActive:      req.OnlyActive,
		UserIDs:         req.UserIDs,
		CreatedBy:       createdBy,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// GetBatch 单个批次
// GET /api/v1/admin/reset-cards/batches/:id
func (h *ResetCardHandler) GetBatch(c *gin.Context) {
	id, ok := parseResetCardID(c)
	if !ok {
		return
	}
	result, err := h.resetCardService.GetBatch(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// ListBatchCards 批次里的卡
// GET /api/v1/admin/reset-cards/batches/:id/cards
func (h *ResetCardHandler) ListBatchCards(c *gin.Context) {
	id, ok := parseResetCardID(c)
	if !ok {
		return
	}
	page, pageSize := parseResetCardPagination(c)
	items, total, err := h.resetCardService.ListBatchCards(c.Request.Context(), id, service.ResetCardAdminCardFilter{
		Status: c.Query("status"),
		Search: c.Query("search"),
	}, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

// RevokeBatch 作废该批次里所有未使用的卡
// POST /api/v1/admin/reset-cards/batches/:id/revoke
func (h *ResetCardHandler) RevokeBatch(c *gin.Context) {
	id, ok := parseResetCardID(c)
	if !ok {
		return
	}
	revoked, err := h.resetCardService.RevokeBatch(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"revoked": revoked})
}

// RevokeCard 作废单张未使用的卡
// POST /api/v1/admin/reset-cards/cards/:id/revoke
func (h *ResetCardHandler) RevokeCard(c *gin.Context) {
	id, ok := parseResetCardID(c)
	if !ok {
		return
	}
	revoked, err := h.resetCardService.RevokeCard(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"revoked": revoked})
}
