package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"strconv"
)

func (h *AccountHandler) SetMonitorPolicyControl(control service.MonitorPolicyControl) {
	h.monitorPolicyControl = control
}
func (h *AccountHandler) DisableMonitorPolicy(c *gin.Context) {
	actor, ok := questionActor(c)
	if !ok {
		return
	}
	if err := h.questionReviews.Authorize(c.Request.Context(), actor); err != nil {
		questionError(c, err)
		return
	}
	id, err := strconv.ParseInt(c.Param("policy"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid policy")
		return
	}
	var input struct {
		Revision *int64 `json:"expected_revision"`
	}
	if !benchmarkDecode(c, &input, 1024) {
		return
	}
	if input.Revision == nil || h.monitorPolicyControl == nil {
		response.BadRequest(c, "Policy revision required")
		return
	}
	if err = h.monitorPolicyControl.DisablePolicy(c.Request.Context(), id, *input.Revision, actor); err != nil {
		response.Error(c, 409, "Policy unavailable or revision changed")
		return
	}
	response.Success(c, gin.H{"enabled": false, "settlement_pending": true})
}
