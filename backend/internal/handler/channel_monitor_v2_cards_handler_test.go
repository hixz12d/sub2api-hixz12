package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type channelMonitorCardsRepoStub struct {
	service.ChannelMonitorV2Repository
	cfg   service.ChannelMonitorV2Config
	query service.ChannelMonitorV2CardsQuery
	calls int
}

func (r *channelMonitorCardsRepoStub) GetConfig(context.Context) (*service.ChannelMonitorV2Config, error) {
	cfg := r.cfg
	return &cfg, nil
}
func (r *channelMonitorCardsRepoStub) UpdateConfig(_ context.Context, cfg service.ChannelMonitorV2Config, version int) (*service.ChannelMonitorV2Config, error) {
	if version != r.cfg.Version {
		return nil, service.ErrChannelMonitorV2ConfigConflict
	}
	r.cfg = cfg
	return &cfg, nil
}
func (r *channelMonitorCardsRepoStub) GetCards(_ context.Context, q service.ChannelMonitorV2CardsQuery, _ service.ChannelMonitorV2Config) (*service.ChannelMonitorV2Cards, error) {
	r.query, r.calls = q, r.calls+1
	return &service.ChannelMonitorV2Cards{Items: []service.ChannelMonitorV2StatusCard{}}, nil
}

func TestChannelMonitorV2CardsHandlerScopeAndInvalidQueries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, query           string
		detail, authenticated bool
		status, calls         int
	}{
		{"user cannot set admin", "?admin=true&allowed_group_ids=99", false, true, 200, 1},
		{"negative page", "?page=-1", false, true, 400, 0},
		{"large page", "?page=1000001", false, true, 400, 0},
		{"large page size", "?page_size=101", false, true, 400, 0},
		{"invalid group", "?group_id=0", false, true, 400, 0},
		{"invalid timestamp", "?as_of=2026-01-01", false, true, 400, 0},
		{"future timestamp", "?as_of=9999-01-01T00:00:00Z", false, true, 400, 0},
		{"missing detail identity", "", true, true, 400, 0},
		{"missing detail", "?platform=openai&group_id=99&model=secret", true, true, 404, 1},
		{"unauthenticated", "", false, false, 401, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &channelMonitorCardsRepoStub{cfg: service.ChannelMonitorV2Config{Enabled: true}}
			h := &ChannelMonitorV2Handler{service: service.NewChannelMonitorV2Service(r), apiKeyService: &channelMonitorV2GroupAuthorizerStub{groups: []service.Group{{ID: 7}}}}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/channel-monitor-v2/cards"+tc.query, nil)
			if tc.authenticated {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
			}
			h.cards(c, false, tc.detail)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, tc.calls, r.calls)
			if r.calls > 0 {
				require.True(t, r.query.Filter.RestrictGroups)
				require.Equal(t, []int64{7}, r.query.Filter.AllowedGroupIDs)
				require.False(t, r.query.IncludeAdmin)
			}
		})
	}
}

func TestChannelMonitorV2CardsHandlerAuthorizationFailure(t *testing.T) {
	r := &channelMonitorCardsRepoStub{cfg: service.ChannelMonitorV2Config{Enabled: true}}
	h := &ChannelMonitorV2Handler{service: service.NewChannelMonitorV2Service(r), apiKeyService: &channelMonitorV2GroupAuthorizerStub{err: errors.New("authorization unavailable")}}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/channel-monitor-v2/cards", nil)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	h.Cards(c)
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.Zero(t, r.calls)
}

func TestChannelMonitorV2ConfigLegacyUpdatePreservesCardSettings(t *testing.T) {
	warning, critical := int64(1000), int64(5000)
	for _, explicit := range []bool{false, true} {
		cfg := service.ChannelMonitorV2Config{Enabled: true, Version: 7, StatusCardSettings: service.ChannelMonitorV2StatusCardSettings{TTFTP90WarningMs: &warning, TTFTP90CriticalMs: &critical}}
		r := &channelMonitorCardsRepoStub{cfg: cfg}
		h := &ChannelMonitorV2Handler{service: service.NewChannelMonitorV2Service(r)}
		body := `{"version":7,"enabled":true}`
		if explicit {
			body = `{"version":7,"enabled":true,"status_card_settings":{"ttft_p90_warning_ms":null,"ttft_p90_critical_ms":null}}`
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPut, "/config", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		h.UpdateConfig(c)
		require.Equal(t, http.StatusOK, w.Code)
		if explicit {
			require.Nil(t, r.cfg.StatusCardSettings.TTFTP90WarningMs)
		} else {
			require.Equal(t, warning, *r.cfg.StatusCardSettings.TTFTP90WarningMs)
		}
	}
}

func TestChannelMonitorV2CardsAdminFieldsNeverSerializeInPublicResponse(t *testing.T) {
	card := service.ChannelMonitorV2StatusCard{Source: "real_traffic"}
	out := &service.ChannelMonitorV2Cards{Items: []service.ChannelMonitorV2StatusCard{card}}
	out.AppendAdminCard(card, service.ChannelMonitorV2AdminCardMetrics{H24: service.ChannelMonitorV2Metric{RequestCount: 100}})
	public, err := json.Marshal(out)
	require.NoError(t, err)
	require.NotContains(t, string(public), "request_count")
	admin, err := json.Marshal(out.AdminResponse())
	require.NoError(t, err)
	require.Contains(t, string(admin), `"request_count":100`)
}
