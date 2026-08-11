package admin

import (
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminAPIKeyHandler handles admin API key management
type AdminAPIKeyHandler struct {
	adminService service.AdminService
}

// NewAdminAPIKeyHandler creates a new admin API key handler
func NewAdminAPIKeyHandler(adminService service.AdminService) *AdminAPIKeyHandler {
	return &AdminAPIKeyHandler{
		adminService: adminService,
	}
}

// AdminUpdateAPIKeyGroupRequest represents the request to update an API key.
type AdminUpdateAPIKeyGroupRequest struct {
	GroupID             *int64     `json:"group_id"`               // nil=不修改, 0=解绑, >0=绑定到目标分组
	ExpectedGroupID     *int64     `json:"expected_group_id"`      // nil=兼容旧式无条件更新, 0=期望未绑定, >0=期望当前分组
	ExpectedUpdatedAt   *time.Time `json:"expected_updated_at"`    // optional immutable snapshot guard for migration tooling
	ResetRateLimitUsage *bool      `json:"reset_rate_limit_usage"` // true=重置 5h/1d/7d 限速用量
}

// UpdateGroup handles updating an API key's admin-managed fields.
// PUT /api/v1/admin/api-keys/:id
func (h *AdminAPIKeyHandler) UpdateGroup(c *gin.Context) {
	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid API key ID")
		return
	}

	var req AdminUpdateAPIKeyGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if req.ExpectedGroupID != nil && req.GroupID == nil {
		response.BadRequest(c, "expected_group_id requires group_id")
		return
	}
	if req.ExpectedUpdatedAt != nil && req.ExpectedGroupID == nil {
		response.BadRequest(c, "expected_updated_at requires expected_group_id")
		return
	}
	if req.ExpectedGroupID != nil && *req.ExpectedGroupID < 0 {
		response.BadRequest(c, "expected_group_id must be non-negative")
		return
	}
	if req.ExpectedGroupID != nil && req.ResetRateLimitUsage != nil && *req.ResetRateLimitUsage {
		response.BadRequest(c, "expected_group_id cannot be combined with reset_rate_limit_usage")
		return
	}

	var resetKey *service.APIKey
	if req.ResetRateLimitUsage != nil && *req.ResetRateLimitUsage {
		resetKey, err = h.adminService.AdminResetAPIKeyRateLimitUsage(c.Request.Context(), keyID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}

	var result *service.AdminUpdateAPIKeyGroupIDResult
	if req.ExpectedGroupID != nil {
		result, err = h.adminService.AdminUpdateAPIKeyGroupIDWithExpected(c.Request.Context(), keyID, req.GroupID, req.ExpectedGroupID, req.ExpectedUpdatedAt)
	} else {
		result, err = h.adminService.AdminUpdateAPIKeyGroupID(c.Request.Context(), keyID, req.GroupID)
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if resetKey != nil && req.GroupID == nil {
		result.APIKey = resetKey
	}

	resp := struct {
		APIKey                 *dto.APIKey `json:"api_key"`
		AutoGrantedGroupAccess bool        `json:"auto_granted_group_access"`
		GrantedGroupID         *int64      `json:"granted_group_id,omitempty"`
		GrantedGroupName       string      `json:"granted_group_name,omitempty"`
	}{
		APIKey:                 dto.APIKeyFromService(result.APIKey),
		AutoGrantedGroupAccess: result.AutoGrantedGroupAccess,
		GrantedGroupID:         result.GrantedGroupID,
		GrantedGroupName:       result.GrantedGroupName,
	}
	response.Success(c, resp)
}
