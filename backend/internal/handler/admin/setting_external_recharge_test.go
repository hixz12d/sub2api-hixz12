//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestExternalRechargeSettingsSaveAndDisable(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})
	for _, enabled := range []bool{true, false, true} {
		rec := doUpdateSettings(t, h, map[string]any{
			"custom_menu_items": []map[string]any{{
				"id": "ldxp-recharge", "label": "Shop", "url": "https://example.com/shop",
				"visibility": "user", "placement": "recharge", "enabled": enabled,
			}},
		}, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var stored []dto.CustomMenuItem
		require.NoError(t, json.Unmarshal([]byte(repo.values[service.SettingKeyCustomMenuItems]), &stored))
		require.Len(t, stored, 1)
		require.NotNil(t, stored[0].Enabled)
		require.Equal(t, enabled, *stored[0].Enabled)
		require.Equal(t, "recharge", stored[0].Placement)
		require.Equal(t, "https://example.com/shop", stored[0].URL)
		injection, err := h.settingService.GetPublicSettingsForInjection(context.Background())
		require.NoError(t, err)
		publicItems := dto.ParseCustomMenuItems(string(injection.(*service.PublicSettingsInjectionPayload).CustomMenuItems))
		if enabled {
			require.Len(t, publicItems, 1)
		} else {
			require.Empty(t, publicItems, "disabled shop must disappear from injected public settings too")
		}
	}
}

func TestExternalRechargeSettingsRejectInvalidPlacement(t *testing.T) {
	for _, tc := range []struct{ placement, visibility, url string }{
		{"invalid", "user", "https://example.com/shop"},
		{"recharge", "admin", "https://example.com/shop"},
		{"recharge", "user", "md:shop"},
	} {
		h, _ := newStepUpSwitchTestHandler(t, map[string]string{})
		rec := doUpdateSettings(t, h, map[string]any{
			"custom_menu_items": []map[string]any{{
				"id": "shop", "label": "Shop", "url": tc.url,
				"visibility": tc.visibility, "placement": tc.placement,
			}},
		}, nil)
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}
