package admin

import (
	"context"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *ProxyHandler) ListGroups(c *gin.Context) {
	groups, err := h.adminService.ListProxyGroups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, groups)
}

func (h *ProxyHandler) SaveGroup(c *gin.Context) {
	var req struct {
		Name                string  `json:"name" binding:"required,max=100"`
		MaxAccountsPerProxy int     `json:"max_accounts_per_proxy" binding:"required,min=1,max=10000"`
		ProxyIDs            []int64 `json:"proxy_ids" binding:"max=10000,dive,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	group := service.ProxyGroup{Name: req.Name, MaxAccountsPerProxy: req.MaxAccountsPerProxy, ProxyIDs: req.ProxyIDs}
	if raw := c.Param("id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid proxy group ID")
			return
		}
		group.ID = id
	}
	executeAdminIdempotentJSON(c, "admin.proxy-groups.save", struct {
		ID    int64
		Input any
	}{group.ID, req}, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		if err := h.adminService.SaveProxyGroup(ctx, &group); err != nil {
			return nil, err
		}
		return group, nil
	})
}

func (h *ProxyHandler) DeleteGroup(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid proxy group ID")
		return
	}
	if err := h.adminService.DeleteProxyGroup(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Proxy group deleted; account egress assignments preserved"})
}
