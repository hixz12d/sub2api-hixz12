package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorV2CardsTimelineAndWindowAggregation(t *testing.T) {
	asOf := time.Date(2026, 8, 8, 12, 5, 0, 0, time.UTC)
	a := newChannelMonitorV2CardAccumulator()
	start := asOf.Add(-24 * time.Hour)
	for i := 0; i < 4; i++ {
		at := start.Add(time.Duration(i) * 5 * time.Minute)
		for _, acc := range a.at(at, asOf) {
			acc.addFact(channelMonitorV2Fact{Success: 1})
		}
	}
	for _, acc := range a.at(start.Add(25*time.Minute), asOf) {
		acc.addFact(channelMonitorV2Fact{Success: 1, Errors: 95})
	}
	for _, acc := range a.at(start.Add(-time.Hour), asOf) {
		acc.addFact(channelMonitorV2Fact{Success: 100})
	}
	require.Empty(t, a.at(asOf, asOf))
	require.Empty(t, a.at(asOf.Add(-8*24*time.Hour), asOf))
	card := a.card(service.ChannelMonitorV2CardIdentity{Platform: "openai", GroupID: 7, Model: "model"}, "Public group", asOf, asOf.Add(-7*24*time.Hour), service.ChannelMonitorV2Config{})
	require.Len(t, card.Timeline, 72)
	require.Equal(t, start, card.Timeline[0].Start)
	require.Equal(t, asOf, card.Timeline[71].End)
	for i := 1; i < 72; i++ {
		require.Equal(t, card.Timeline[i-1].End, card.Timeline[i].Start)
	}
	require.InDelta(t, .05, *card.Windows.H24.SuccessRate, 1e-9)
	require.InDelta(t, .525, *card.Windows.D7.SuccessRate, 1e-9)
	require.Equal(t, 1.0, *card.Timeline[0].SuccessRate)
	require.Nil(t, card.Timeline[2].SuccessRate)
	require.Equal(t, "no_data", card.Timeline[2].State)
	require.Equal(t, "no_data", card.Current.RequestState)
	require.Nil(t, card.Capability)
	require.Nil(t, card.Windows.H24.CacheReadRatio)
	require.True(t, card.Windows.D7.CoverageComplete)
}

func TestChannelMonitorV2CardsEmptyScopeHasNoDatabaseCalls(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	r := &channelMonitorV2Repository{db: db}
	out, err := r.GetCards(context.Background(), service.ChannelMonitorV2CardsQuery{Filter: service.ChannelMonitorV2Filter{RestrictGroups: true}, Page: 1, PageSize: 20}, service.ChannelMonitorV2Config{})
	require.NoError(t, err)
	require.Empty(t, out.Items)
	require.Nil(t, out.AsOf)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChannelMonitorV2CardsNoWatermark(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT w.usage_coverage_start").WillReturnRows(sqlmock.NewRows([]string{"usage", "errors", "through", "computed", "upgrade"}))
	mock.ExpectCommit()
	out, err := (&channelMonitorV2Repository{db: db}).GetCards(context.Background(), service.ChannelMonitorV2CardsQuery{Page: 1, PageSize: 20}, service.ChannelMonitorV2Config{})
	require.NoError(t, err)
	require.Nil(t, out.AsOf)
	require.Nil(t, out.ComputedAt)
	require.True(t, out.Stale)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChannelMonitorV2CardsBatchQueriesAndPrivacy(t *testing.T) {
	for _, count := range []int{1, 20, 100} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			now := time.Date(2026, 8, 8, 12, 5, 0, 0, time.UTC)
			asOf := now.Add(-20 * time.Minute)
			start := asOf.Add(-7 * 24 * time.Hour)
			cfg := service.ChannelMonitorV2Config{RefreshIntervalSeconds: 300, Platforms: []service.ChannelMonitorV2PlatformConfig{{Platform: "openai", Enabled: true}}, GroupIDs: []int64{7}}
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT w.usage_coverage_start").WillReturnRows(sqlmock.NewRows([]string{"usage", "errors", "through", "computed", "upgrade"}).AddRow(start, start, asOf, now, start))
			dims := sqlmock.NewRows([]string{"platform", "group_id", "name", "display_model"})
			facts := sqlmock.NewRows([]string{"at", "platform", "group_id", "model", "success", "errors", "input", "output", "creation", "read", "ttft_sum", "ttft_count", "duration_sum", "duration_count", "observed_total", "observed_cached", "observed_measured"})
			hist := sqlmock.NewRows([]string{"at", "platform", "group_id", "model", "metric", "bound", "count"})
			for i := 0; i < count; i++ {
				model := fmt.Sprintf("model-%03d", i)
				dims.AddRow("openai", 7, "Public group", model)
				facts.AddRow(asOf.Add(-5*time.Minute), "openai", 7, model, 90, 10, 10, 50, 0, 0, 90000, 90, 180000, 90, 1000, 100, 50)
				hist.AddRow(asOf.Add(-5*time.Minute), "openai", 7, model, "ttft", 1000, 90)
			}
			dims.AddRow("openai", 7, "Public group", "next-page")
			mock.ExpectQuery("SELECT m.platform,m.group_id,g.name").WithArgs(start, asOf, sqlmock.AnyArg(), sqlmock.AnyArg(), 300, count+1, 0).WillReturnRows(dims)
			mock.ExpectQuery("SELECT m.bucket_start,m.platform,m.group_id,selected.model, SUM\\(m.success_requests\\)").WithArgs(start, asOf, sqlmock.AnyArg(), sqlmock.AnyArg(), 300, sqlmock.AnyArg()).WillReturnRows(facts)
			mock.ExpectQuery("SELECT m.bucket_start,m.platform,m.group_id,selected.model,m.metric").WithArgs(start, asOf, sqlmock.AnyArg(), sqlmock.AnyArg(), 300, sqlmock.AnyArg()).WillReturnRows(hist)
			mock.ExpectQuery("SELECT observation_v1_collection_start").WillReturnRows(sqlmock.NewRows([]string{"start", "through"}).AddRow(start, asOf))
			mock.ExpectQuery("SELECT m.bucket_start,m.platform,m.group_id,selected.model,m.bucket_index").WithArgs(start, asOf, sqlmock.AnyArg(), sqlmock.AnyArg(), 300, sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"at", "platform", "group", "model", "bucket", "count"}))
			mock.ExpectCommit()
			out, err := (&channelMonitorV2Repository{db: db}).GetCards(context.Background(), service.ChannelMonitorV2CardsQuery{Page: 1, PageSize: count, ServerNow: now, Filter: service.ChannelMonitorV2Filter{RestrictGroups: true, AllowedGroupIDs: []int64{7}}}, cfg)
			require.NoError(t, err)
			require.Len(t, out.Items, count)
			require.True(t, out.HasMore)
			require.True(t, out.CoverageComplete)
			require.True(t, out.Stale)
			require.Equal(t, asOf, *out.AsOf)
			for _, card := range out.Items {
				require.InDelta(t, .9, *card.Windows.H24.SuccessRate, 1e-9)
				require.InDelta(t, .1, *card.Windows.H24.ObservedCacheReadRatio, 1e-9)
				require.Equal(t, int64(1000), *card.Windows.H24.TTFTP90Ms)
				require.Equal(t, "partial_failure", card.Current.RequestState)
			}
			body, err := json.Marshal(out)
			require.NoError(t, err)
			for _, forbidden := range []string{"request_count", "success_requests", "sample_count", "account_id", "api_key", "next-page"} {
				require.NotContains(t, string(body), forbidden)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestChannelMonitorV2CardsPartialCoverageAndZeroFailures(t *testing.T) {
	asOf := time.Date(2026, 8, 8, 12, 5, 0, 0, time.UTC)
	a := newChannelMonitorV2CardAccumulator()
	for _, acc := range a.at(asOf.Add(-5*time.Minute), asOf) {
		acc.addFact(channelMonitorV2Fact{Errors: 50})
	}
	card := a.card(service.ChannelMonitorV2CardIdentity{}, "", asOf, asOf.Add(-time.Hour), service.ChannelMonitorV2Config{})
	require.False(t, card.Windows.D7.CoverageComplete)
	require.NotNil(t, card.Windows.H24.SuccessRate)
	require.Zero(t, *card.Windows.H24.SuccessRate)
	require.Equal(t, "many_failures", card.Current.RequestState)
	require.Nil(t, card.Windows.H24.TTFTP90Ms)
}
