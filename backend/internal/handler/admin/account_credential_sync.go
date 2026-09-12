package admin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type oauthSyncControl interface {
	InstanceID(context.Context) (string, error)
	Submit(context.Context, string, int64, *service.SyncOAuthCredentialsRequest) (*service.OAuthSyncOperation, bool, error)
	Operation(context.Context, string, int64, string) (*service.OAuthSyncOperation, error)
	Receipt(context.Context, *service.OAuthSyncOperation) map[string]any
	Snapshot(context.Context, string, int64) (map[string]any, error)
	Retry(context.Context, string, int64, string, string) (*service.OAuthSyncOperation, error)
}

func (h *AccountHandler) SetOAuthSyncService(svc *service.OAuthSyncService) { h.oauthSync = svc }
func oauthSyncScope(c *gin.Context, accountID int64) string {
	return "oauth-sync:" + adminActorScope(c) + ":" + strconv.FormatInt(accountID, 10)
}
func (h *AccountHandler) IntegrationCapabilities(c *gin.Context) {
	instance := ""
	if h.oauthSync != nil {
		instance, _ = h.oauthSync.InstanceID(c.Request.Context())
	}
	ready := instance != ""
	handoff := false
	if provider, ok := h.oauthSync.(interface{ RefreshHandoffSupported() bool }); ok {
		handoff = ready && provider.RefreshHandoffSupported()
	}
	_, readback := h.oauthSync.(oauthSyncAccessReader)
	recovery := false
	if provider, ok := h.oauthSync.(interface{ AuthRecoverySupported() bool }); ok {
		recovery = ready && provider.AuthRecoverySupported()
	}
	modes := []string{"credentials_only"}
	if recovery {
		modes = append(modes, "auth_only")
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{
		"schema_version": 1, "instance_id": instance, "contracts": gin.H{"oauth_sync": []int{1}},
		"oauth_sync": gin.H{
			"revision": 5, "available": ready, "credential_cas": true,
			"refresh_handoff": handoff, "refresh_fencing": handoff, "refresh_fencing_scope": "observed_rt_lineage",
			"operation_receipts": ready, "atomic_receipts": ready, "resumable_followups": ready,
			"remote_state": ready, "recovery_modes": modes,
			"auth_only": recovery, "candidate_validation": recovery, "versioned_auth_errors": recovery,
			"validation_scope": service.OAuthValidationScope, "single_writer": false, "metadata_changes": false,
			"instance_identity": ready, "scheduler_confirmation": "shared_projection_only",
			"access_token_readback": ready && readback,
		},
	})
}
func (h *AccountHandler) oauthSyncTarget(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return 0, false
	}
	if h.oauthSync == nil {
		response.ErrorFrom(c, service.ErrIdempotencyStoreUnavail)
		return 0, false
	}
	c.Header("Cache-Control", "no-store")
	return id, true
}
func decodeOAuthSyncBody(c *gin.Context, target any, size int64) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, size))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		response.BadRequest(c, "Invalid sync request")
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		response.BadRequest(c, "Invalid sync request")
		return false
	}
	return true
}
func (h *AccountHandler) SyncOAuthCredentials(c *gin.Context) {
	id, ok := h.oauthSyncTarget(c)
	if !ok {
		return
	}
	var req service.SyncOAuthCredentialsRequest
	if !decodeOAuthSyncBody(c, &req, 64<<10) {
		return
	}
	if req.ContractVersion == 0 {
		req.ContractVersion = 1
	}
	op, replay, err := h.oauthSync.Submit(c.Request.Context(), oauthSyncScope(c, id), id, &req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if replay {
		c.Header("X-Idempotency-Replayed", "true")
	}
	response.Success(c, h.oauthSync.Receipt(c.Request.Context(), op))
}
func (h *AccountHandler) GetOAuthSyncOperation(c *gin.Context) {
	id, ok := h.oauthSyncTarget(c)
	if !ok {
		return
	}
	key, err := service.NormalizeIdempotencyKey(c.Param("operation_id"))
	if err != nil || key == "" {
		response.ErrorFrom(c, service.ErrOAuthSyncInvalid)
		return
	}
	op, err := h.oauthSync.Operation(c.Request.Context(), oauthSyncScope(c, id), id, key)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if op == nil {
		// Preserve read access to first-batch receipts; never resume their writes.
		if service.DefaultIdempotencyCoordinator() != nil {
			legacy, err := service.LookupOAuthSyncOperation(c.Request.Context(), oauthSyncScope(c, id), key)
			if err != nil {
				response.ErrorFrom(c, err)
				return
			}
			legacy["source"] = "legacy"
			response.Success(c, legacy)
			return
		}
		response.Success(c, gin.H{"state": "unknown", "operation_id": key})
		return
	}
	response.Success(c, gin.H{"state": "recorded", "operation_id": key, "receipt": h.oauthSync.Receipt(c.Request.Context(), op)})
}
func (h *AccountHandler) GetOAuthSyncState(c *gin.Context) {
	id, ok := h.oauthSyncTarget(c)
	if !ok {
		return
	}
	snapshot, err := h.oauthSync.Snapshot(c.Request.Context(), oauthSyncScope(c, id), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, snapshot)
}
func (h *AccountHandler) RetryOAuthSyncOperation(c *gin.Context) {
	id, ok := h.oauthSyncTarget(c)
	if !ok {
		return
	}
	key, err := service.NormalizeIdempotencyKey(c.Param("operation_id"))
	if err != nil || key == "" {
		response.ErrorFrom(c, service.ErrOAuthSyncInvalid)
		return
	}
	var req struct {
		ExpectedInstanceID string `json:"expected_instance_id"`
	}
	if !decodeOAuthSyncBody(c, &req, 1024) {
		return
	}
	op, err := h.oauthSync.Retry(c.Request.Context(), oauthSyncScope(c, id), id, key, req.ExpectedInstanceID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"state": "recorded", "operation_id": key, "receipt": h.oauthSync.Receipt(c.Request.Context(), op)})
}
