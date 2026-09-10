package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"slices"
	"strconv"
)

func (h *AccountHandler) policyManager(c *gin.Context) (service.MonitorPolicyManagement, int64, bool) {
	actor, ok := questionActor(c)
	if !ok {
		return nil, 0, false
	}
	if err := h.questionReviews.Authorize(c.Request.Context(), actor); err != nil {
		questionError(c, err)
		return nil, 0, false
	}
	manager, ok := h.monitorPolicyControl.(service.MonitorPolicyManagement)
	if !ok {
		response.Error(c, 503, "Policy management unavailable")
		return nil, 0, false
	}
	return manager, actor, true
}
func (h *AccountHandler) MonitorPolicies(c *gin.Context) {
	manager, _, ok := h.policyManager(c)
	if !ok {
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 201 {
		response.BadRequest(c, "Invalid page")
		return
	}
	items, err := manager.ListPolicies(c.Request.Context(), 50, (page-1)*50)
	if err != nil {
		response.InternalError(c, "Policies unavailable")
		return
	}
	response.Success(c, gin.H{"items": items, "page": page, "has_more": len(items) == 50})
}
func (h *AccountHandler) SaveMonitorPolicy(c *gin.Context) {
	manager, actor, ok := h.policyManager(c)
	if !ok {
		return
	}
	var input struct {
		GroupID      int64                         `json:"group_id"`
		DisplayName  string                        `json:"display_name"`
		PrimaryModel string                        `json:"primary_model"`
		ExtraModels  []string                      `json:"extra_models"`
		Enabled      bool                          `json:"enabled"`
		Probe        service.GroupProbeConfig      `json:"probe_config"`
		Capability   service.GroupCapabilityConfig `json:"capability_config"`
		Revision     *int64                        `json:"expected_revision"`
	}
	input.Probe = service.DefaultGroupProbeConfig()
	input.Capability = service.DefaultGroupCapabilityConfig()
	if !benchmarkDecode(c, &input, 65536) {
		return
	}
	if input.Revision == nil {
		response.BadRequest(c, "Expected revision required")
		return
	}
	p := service.ChannelMonitorGroupPolicy{GroupID: input.GroupID, DisplayName: input.DisplayName, PrimaryModel: input.PrimaryModel, ExtraModels: input.ExtraModels, Enabled: input.Enabled, ProbeConfig: input.Probe, CapabilityConfig: input.Capability}
	if raw := c.Param("policy"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid policy")
			return
		}
		p.ID = id
	}
	if err := p.Normalize(); err != nil {
		response.BadRequest(c, "Invalid policy configuration")
		return
	}
	if p.Enabled && (p.CapabilityConfig.Enabled || p.ProbeConfig.Enabled) {
		if h.detectorTasks == nil || h.detectorTasks.Executor == nil || h.detectorTasks.Executor.Scheduler == nil {
			response.Error(c, 503, "Platform execution is not admitted")
			return
		}
		resolver := h.detectorTasks.Executor.Scheduler.Resolver
		if resolver == nil || !slices.Contains(resolver.AllowedGroupIDs, p.GroupID) {
			response.Forbidden(c, "Group is not admitted")
			return
		}
		flags := resolver.Flags.GetMonitorFeatureFlags(c.Request.Context())
		if p.CapabilityConfig.Enabled && !flags.ScheduledDetectionAllowed(true) || p.ProbeConfig.Enabled && !flags.GroupProbeAllowed() {
			response.Forbidden(c, "Execution is disabled")
			return
		}
	}
	result, err := manager.SavePolicy(c.Request.Context(), p, *input.Revision, actor)
	if err != nil {
		response.Error(c, 409, "Policy unavailable or revision changed")
		return
	}
	response.Success(c, result)
}
func (h *AccountHandler) DeleteMonitorPolicy(c *gin.Context) {
	manager, actor, ok := h.policyManager(c)
	if !ok {
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
	if input.Revision == nil {
		response.BadRequest(c, "Expected revision required")
		return
	}
	if err = manager.DeletePolicy(c.Request.Context(), id, *input.Revision, actor); err != nil {
		response.Error(c, 409, "Policy unavailable or revision changed")
		return
	}
	response.Success(c, gin.H{"deleted": true, "settlement_pending": true})
}
