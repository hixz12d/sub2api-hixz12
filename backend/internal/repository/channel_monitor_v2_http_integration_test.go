//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Uses locally signed JWTs and real user/group repositories against the fixture.
// Login endpoints and production route registration are outside this test.
func verifyMonitorCardsHTTPPostgres(t *testing.T, publicGroup, privateGroup int64) {
	t.Helper()
	ctx := context.Background()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{Email: "monitor-http-" + uuid.NewString() + "@example.com"})
	_, err := integrationDB.ExecContext(ctx, `UPDATE groups SET is_exclusive=true WHERE id=$1`, privateGroup)
	require.NoError(t, err)
	keys := service.NewAPIKeyService(nil, NewUserRepository(client, integrationDB), NewGroupRepository(client, integrationDB), NewUserSubscriptionRepository(client), nil, nil, nil)
	h := handler.NewChannelMonitorV2Handler(service.NewChannelMonitorV2Service(&channelMonitorV2Repository{db: integrationDB}), keys)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	cfg := &config.Config{}
	cfg.JWT.Secret = "monitor-test-" + uuid.NewString()
	cfg.JWT.AccessTokenExpireMinutes = 60
	users := NewUserRepository(client, integrationDB)
	auth := service.NewAuthService(nil, users, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	token, err := auth.GenerateToken(ctx, user)
	require.NoError(t, err)
	router.Use(gin.HandlerFunc(middleware.NewJWTAuthMiddleware(auth, service.NewUserService(users, nil, nil, nil), nil, nil)))
	router.GET("/cards", h.Cards)
	router.GET("/cards/detail", h.CardDetail)
	for _, tc := range []struct {
		name, path    string
		authenticated bool
		status, items int
	}{
		{"anonymous", "/cards", false, 401, 0},
		{"public", "/cards", true, 200, 2},
		{"forged scope", fmt.Sprintf("/cards?admin=true&allowed_group_ids=%d&group_id=%d", privateGroup, privateGroup), true, 200, 0},
		{"private detail", fmt.Sprintf("/cards/detail?platform=openai&group_id=%d&model=visible", privateGroup), true, 404, 0},
		{"public detail", fmt.Sprintf("/cards/detail?platform=openai&group_id=%d&model=__other__", publicGroup), true, 200, 1},
	} {
		t.Run("http/"+tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.authenticated {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			for _, forbidden := range []string{"cards-private", "request_count", "success_requests", "sample_count", "account_id", "api_key", "monitor_cache_measured_requests"} {
				require.NotContains(t, w.Body.String(), forbidden)
			}
			if tc.status == 200 {
				var body struct {
					Data struct {
						Items []service.ChannelMonitorV2StatusCard `json:"items"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				require.Len(t, body.Data.Items, tc.items)
				for _, card := range body.Data.Items {
					require.Equal(t, publicGroup, card.Identity.GroupID)
					require.NotNil(t, card.Windows.H24.ObservationEvidence)
					if card.Identity.Model == "__other__" {
						require.InDelta(t, .1, *card.Windows.H24.ObservedCacheReadRatio, 1e-9)
						require.Equal(t, "low_sample", card.Windows.H24.ObservationEvidence.Cache)
					}
				}
			}
		})
	}
	t.Run("http/grant-revoke-and-public-restriction", func(t *testing.T) {
		requestCount := func(expected int, forbidden string) {
			t.Helper()
			req := httptest.NewRequest(http.MethodGet, "/cards", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, 200, w.Code, w.Body.String())
			var body struct {
				Data struct {
					Items []service.ChannelMonitorV2StatusCard `json:"items"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Len(t, body.Data.Items, expected)
			if forbidden != "" {
				require.NotContains(t, w.Body.String(), forbidden)
			}
		}
		_, err := integrationDB.ExecContext(ctx, `INSERT INTO user_allowed_groups(user_id,group_id) VALUES($1,$2)`, user.ID, privateGroup)
		require.NoError(t, err)
		requestCount(4, "")
		_, err = integrationDB.ExecContext(ctx, `DELETE FROM user_allowed_groups WHERE user_id=$1 AND group_id=$2`, user.ID, privateGroup)
		require.NoError(t, err)
		requestCount(2, "cards-private")
		_, err = integrationDB.ExecContext(ctx, `UPDATE users SET restrict_public_groups=true WHERE id=$1`, user.ID)
		require.NoError(t, err)
		requestCount(0, "cards-public")
	})
	t.Run("http/invalid-and-revoked-jwt", func(t *testing.T) {
		assertRejected := func(bearer string) {
			t.Helper()
			req := httptest.NewRequest(http.MethodGet, "/cards", nil)
			req.Header.Set("Authorization", "Bearer "+bearer)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.NotContains(t, w.Body.String(), "cards-public")
		}
		assertRejected("not-a-jwt")
		// Token version is derived from the persisted password hash in this schema;
		// password mutation/revocation is covered by AuthService tests. Keep the
		// HTTP fixture focused on the route's JWT rejection boundary.
	})
}
