package admin

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type oauthSyncAccessReader interface {
	ReadOAuthSyncAccessToken(context.Context, int64, string, int64, string) (map[string]any, error)
}

func (h *AccountHandler) ReadOAuthSyncAccessToken(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	// The bridge key stays on the Team server. Browser JWT sessions cannot read ATs here.
	if c.GetString("auth_method") != "admin_api_key" {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "message": "Admin API key required"})
		return
	}
	id, ok := h.oauthSyncTarget(c)
	if !ok {
		return
	}
	reader, ok := h.oauthSync.(oauthSyncAccessReader)
	if !ok {
		response.ErrorFrom(c, service.ErrIdempotencyStoreUnavail)
		return
	}
	var req struct {
		ExpectedInstanceID string `json:"expected_instance_id"`
		ExpectedVersion    int64  `json:"expected_credential_version"`
		ExpectedUpdatedAt  string `json:"expected_updated_at"`
	}
	if !decodeOAuthSyncBody(c, &req, 2048) {
		return
	}
	result, err := reader.ReadOAuthSyncAccessToken(c.Request.Context(), id, req.ExpectedInstanceID, req.ExpectedVersion, req.ExpectedUpdatedAt)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
