package admin

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

type oauthRefreshHandoffService interface {
	RefreshHandoff(context.Context, string, int64, *service.OpenAIRefreshHandoffRequest) (map[string]any, error)
}

func (h *AccountHandler) OAuthRefreshHandoff(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if c.GetString("auth_method") != "admin_api_key" {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "message": "Admin API key required"})
		return
	}
	id, ok := h.oauthSyncTarget(c)
	if !ok {
		return
	}
	svc, ok := h.oauthSync.(oauthRefreshHandoffService)
	if !ok {
		response.ErrorFrom(c, service.ErrOpenAIRefreshFenced)
		return
	}
	var req service.OpenAIRefreshHandoffRequest
	if !decodeOAuthSyncBody(c, &req, 4096) {
		return
	}
	result, err := svc.RefreshHandoff(c.Request.Context(), oauthSyncScope(c, id), id, &req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
