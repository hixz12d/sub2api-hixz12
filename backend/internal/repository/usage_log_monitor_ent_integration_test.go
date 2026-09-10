//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUsageLogMonitorEntRoundTrip(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)
	user := mustCreateUser(t, client, &service.User{Email: "monitor-" + uuid.NewString() + "@example.com"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "fixture-" + uuid.NewString(), Name: "monitor"})
	account := mustCreateAccount(t, client, &service.Account{Name: "monitor-" + uuid.NewString()})
	origin, method, version := "real_traffic", "visible_stream_v1", 1
	total, cached, visible, generation, rate, first := int64(100), int64(0), int64(80), int64(2000), int64(40000), int64(1000)
	row := &service.UsageLog{UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID, RequestID: uuid.NewString(), Model: "fixture-model", CreatedAt: time.Now(), InputTokens: 100, OutputTokens: 80, TotalCost: 0.3, ActualCost: 0.6, RequestOrigin: &origin, MonitorObservationVersion: &version, MonitorInputTokensTotal: &total, MonitorCacheReadTokens: &cached, MonitorVisibleOutputTokens: &visible, MonitorGenerationMs: &generation, MonitorOutputTPSMilli: &rate, MonitorTPSMethod: &method, MonitorFirstVisibleMs: &first}
	_, err := repo.Create(ctx, row)
	require.NoError(t, err)
	entity, err := client.UsageLog.Get(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, row.RequestOrigin, entity.RequestOrigin)
	require.Equal(t, row.MonitorObservationVersion, entity.MonitorObservationVersion)
	require.Equal(t, row.MonitorInputTokensTotal, entity.MonitorInputTokensTotal)
	require.Equal(t, row.MonitorCacheReadTokens, entity.MonitorCacheReadTokens)
	require.Equal(t, row.MonitorVisibleOutputTokens, entity.MonitorVisibleOutputTokens)
	require.Equal(t, row.MonitorGenerationMs, entity.MonitorGenerationMs)
	require.Equal(t, row.MonitorOutputTPSMilli, entity.MonitorOutputTpsMilli)
	require.Equal(t, row.MonitorTPSMethod, entity.MonitorTpsMethod)
	require.Equal(t, row.MonitorFirstVisibleMs, entity.MonitorFirstVisibleMs)
	require.InDelta(t, 0.6, entity.ActualCost, 1e-10)
	legacy, err := client.UsageLog.Create().SetUserID(user.ID).SetAPIKeyID(key.ID).SetAccountID(account.ID).SetRequestID(uuid.NewString()).SetModel("fixture-model").Save(ctx)
	require.NoError(t, err)
	loaded, err := repo.GetByID(ctx, legacy.ID)
	require.NoError(t, err)
	require.Nil(t, loaded.RequestOrigin)
	require.Nil(t, loaded.MonitorCacheReadTokens)
	require.Nil(t, loaded.MonitorOutputTPSMilli)
}
