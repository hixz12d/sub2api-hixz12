package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"strconv"
)

func (h *AccountHandler) MonitorPolicyReports(c *gin.Context) {
	manager, _, ok := h.policyManager(c)
	if !ok {
		return
	}
	reports, ok := manager.(service.PlatformCapabilityReports)
	if !ok {
		response.Error(c, 503, "Reports unavailable")
		return
	}
	id, err := strconv.ParseInt(c.Param("policy"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid policy")
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 201 {
		response.BadRequest(c, "Invalid page")
		return
	}
	items, err := reports.PolicyReports(c.Request.Context(), id, 50, (page-1)*50)
	if err != nil {
		response.InternalError(c, "Reports unavailable")
		return
	}
	response.Success(c, gin.H{"items": items, "has_more": len(items) == 50, "page": page})
}
