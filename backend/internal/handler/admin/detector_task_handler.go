package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *AccountHandler) SetDetectorTasks(s *service.DetectorTaskService) { h.detectorTasks = s }
func (h *AccountHandler) DetectorPlan(c *gin.Context) {
	actor, ok := questionActor(c)
	if !ok {
		return
	}
	if err := h.questionReviews.Authorize(c.Request.Context(), actor); err != nil {
		questionError(c, err)
		return
	}
	var input struct {
		KeyID   int64  `json:"site_api_key_id"`
		Channel string `json:"channel"`
		Model   string `json:"request_model"`
		Claimed string `json:"claimed_model"`
		Tier    string `json:"tier"`
	}
	if !benchmarkDecode(c, &input, 4096) {
		return
	}
	plan, err := h.detectorTasks.Plan(c.Request.Context(), actor, input.KeyID, input.Channel, input.Model, input.Claimed, input.Tier)
	if err != nil {
		response.Error(c, 503, "Plan unavailable: verify key, benchmark and executor admission")
		return
	}
	response.Success(c, gin.H{"id": plan.ID, "configuration_hash": plan.ConfigurationHash, "planned_requests": plan.MaximumOutboundRequests, "expires_at": plan.ExpiresAt, "estimate_status": plan.EstimateStatus, "benchmark": plan.Benchmark})
}
func (h *AccountHandler) DetectorCreate(c *gin.Context) {
	actor, ok := questionActor(c)
	if !ok {
		return
	}
	if err := h.questionReviews.Authorize(c.Request.Context(), actor); err != nil {
		questionError(c, err)
		return
	}
	var input struct {
		PlanID string `json:"plan_id"`
		Hash   string `json:"configuration_hash"`
		Key    string `json:"idempotency_key"`
	}
	if !benchmarkDecode(c, &input, 4096) {
		return
	}
	id, err := h.detectorTasks.Create(c.Request.Context(), actor, input.PlanID, input.Key, input.Hash)
	if err != nil {
		response.Error(c, 409, "Job not created: plan, permission or budget changed")
		return
	}
	response.Success(c, gin.H{"id": id})
}
func (h *AccountHandler) DetectorJob(c *gin.Context) {
	actor, ok := questionActor(c)
	if !ok {
		return
	}
	if err := h.questionReviews.Authorize(c.Request.Context(), actor); err != nil {
		questionError(c, err)
		return
	}
	if h.detectorTasks == nil || h.detectorTasks.Store == nil {
		response.Error(c, 503, "Detector unavailable")
		return
	}
	job, err := h.detectorTasks.Store.GetJobForOwner(c.Request.Context(), c.Param("job"), actor)
	if err != nil {
		response.NotFound(c, "Job unavailable")
		return
	}
	reports, err := h.detectorTasks.Store.ReportsForOwner(c.Request.Context(), job.ID, actor)
	if err != nil {
		response.InternalError(c, "Report unavailable")
		return
	}
	response.Success(c, gin.H{"reports": reports, "id": job.ID, "state": job.State, "planned_requests": job.BaseRequestsPlanned, "dispatched": job.OutboundDispatched, "completed": job.OutboundCompleted, "created_at": job.CreatedAt, "finished_at": job.FinishedAt})
}
func (h *AccountHandler) DetectorCancel(c *gin.Context) {
	actor, ok := questionActor(c)
	if !ok {
		return
	}
	if err := h.questionReviews.Authorize(c.Request.Context(), actor); err != nil {
		questionError(c, err)
		return
	}
	if h.detectorTasks == nil || h.detectorTasks.Jobs == nil {
		response.Error(c, 503, "Detector unavailable")
		return
	}
	if err := h.detectorTasks.Jobs.CancelForOwner(c.Request.Context(), c.Param("job"), actor); err != nil {
		response.NotFound(c, "Job unavailable")
		return
	}
	response.Success(c, gin.H{"cancel_requested": true})
}
