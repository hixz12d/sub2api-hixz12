package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorV2CardsUpgradeCoverageIsConservative(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Date(2026, 8, 8, 12, 5, 0, 0, time.UTC)
	oldStart := now.Add(-30 * 24 * time.Hour)
	upgradeStart := now.Add(-7 * 24 * time.Hour).Add(12 * time.Second)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT w.usage_coverage_start").WillReturnRows(sqlmock.NewRows([]string{"usage", "errors", "through", "computed", "upgrade"}).AddRow(oldStart, oldStart, now, now, upgradeStart))
	mock.ExpectQuery("SELECT m.platform,m.group_id,g.name").WillReturnRows(sqlmock.NewRows([]string{"platform", "group_id", "name", "model"}))
	mock.ExpectCommit()
	out, err := (&channelMonitorV2Repository{db: db}).GetCards(context.Background(), service.ChannelMonitorV2CardsQuery{Page: 1, PageSize: 20, ServerNow: now}, service.ChannelMonitorV2Config{})
	require.NoError(t, err)
	require.False(t, out.CoverageComplete)
	require.Equal(t, upgradeStart.Truncate(5*time.Minute).Add(5*time.Minute), *out.CoverageStart)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChannelMonitorV2CardsRejectsAsOfBeyondWatermark(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now().UTC().Truncate(5 * time.Minute)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT w.usage_coverage_start").WillReturnRows(sqlmock.NewRows([]string{"usage", "errors", "through", "computed", "upgrade"}).AddRow(now.Add(-time.Hour), now.Add(-time.Hour), now.Add(-5*time.Minute), now, now.Add(-time.Hour)))
	mock.ExpectRollback()
	_, err = (&channelMonitorV2Repository{db: db}).GetCards(context.Background(), service.ChannelMonitorV2CardsQuery{Page: 1, PageSize: 20, ServerNow: now, AsOf: &now}, service.ChannelMonitorV2Config{})
	require.ErrorIs(t, err, service.ErrChannelMonitorV2InvalidRange)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChannelMonitorV2CardsImageModelsOmitTextLatency(t *testing.T) {
	asOf := time.Now().UTC().Truncate(5 * time.Minute)
	a := newChannelMonitorV2CardAccumulator()
	for _, acc := range a.at(asOf.Add(-5*time.Minute), asOf) {
		acc.addFact(channelMonitorV2Fact{Success: 100, TTFTCount: 100, TTFTSum: 100000, DurationCount: 100, DurationSum: 200000})
		acc.addHistogram(channelMonitorV2Histogram{Metric: "ttft", UpperBound: 1000, Count: 100})
		acc.addHistogram(channelMonitorV2Histogram{Metric: "duration", UpperBound: 2000, Count: 100})
	}
	for _, model := range []string{"gpt-image-2", "grok-imagine-image"} {
		card := a.card(service.ChannelMonitorV2CardIdentity{Model: model}, "", asOf, asOf.Add(-7*24*time.Hour), service.ChannelMonitorV2Config{})
		require.Nil(t, card.Windows.H24.TTFTP90Ms)
		require.False(t, card.Windows.H24.Evidence.HasTTFT)
		require.Equal(t, int64(2000), *card.Windows.H24.DurationP50Ms)
		require.Equal(t, "unknown", card.Current.PerformanceState)
		require.Nil(t, card.Capability)
	}
}
