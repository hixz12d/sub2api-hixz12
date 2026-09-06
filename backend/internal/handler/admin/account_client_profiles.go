package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *AccountHandler) GetClientProfiles(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	catalog, err := service.PublicCodexClientCatalog()
	if err != nil {
		response.InternalError(c, "Reviewed client catalog unavailable")
		return
	}
	response.Success(c, catalog)
}

func (h *AccountHandler) PreviewClientProfile(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var input service.CodexProfilePreviewInput
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 128*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		response.BadRequest(c, "Invalid client profile preview")
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		response.BadRequest(c, "Expected one preview document")
		return
	}
	status := "unknown"
	if input.AccountID < 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if input.AccountID > 0 {
		account, err := h.adminService.GetAccount(c.Request.Context(), input.AccountID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		if account == nil || account.Platform != input.Platform || account.Type != input.Type {
			response.BadRequest(c, "Preview account type mismatch")
			return
		}
		status = h.accountTestService.CodexProfilePluginStatus(account)
	}
	response.Success(c, service.PreviewCodexClientProfile(input, status))
}

func (h *AccountHandler) GetEffectiveClientProfile(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	account, err := h.adminService.GetAccount(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	catalog, err := service.PublicCodexClientCatalog()
	if err != nil {
		response.InternalError(c, "Reviewed client catalog unavailable")
		return
	}
	extra := make(map[string]any)
	for _, key := range []string{"codex_relay_mode", "codex_identity_policy_version", "codex_client_profile", "codex_installation_policy", "codex_fingerprint_mode", "codex_relay_shadow_enabled", "enable_tls_fingerprint", "tls_fingerprint_profile_id"} {
		if value, ok := account.Extra[key]; ok {
			extra[key] = value
		}
	}
	input := service.CodexProfilePreviewInput{AccountID: id, Platform: account.Platform, Type: account.Type, Extra: extra,
		Operation: service.CodexOperationResponses, Transport: service.CodexTransportHTTP, CatalogRevision: catalog.Revision}
	response.Success(c, service.PreviewCodexClientProfile(input, h.accountTestService.CodexProfilePluginStatus(account)))
}
