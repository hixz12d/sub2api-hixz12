package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorV2CapabilityBaselineObservedSuccess(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		success, errors, ignored int64
		observed, scored         float64
	}{
		{"ignored errors are not successes", 90, 10, 5, .90, .05},
		{"all failed and ignored", 0, 50, 50, 0, 0},
		{"no requests", 0, 0, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			acc := newMetricAccumulator()
			acc.success, acc.errors = tc.success, tc.errors
			metric := acc.metric(1, false)
			applyIgnoredErrors(&metric, tc.ignored)
			require.Equal(t, tc.success+tc.errors, metric.RequestCount)
			require.InDelta(t, tc.observed, metric.SuccessRate, 1e-9)
			require.InDelta(t, tc.scored, metric.ErrorRate, 1e-9)
		})
	}
}

func TestChannelMonitorV2CapabilityBaselineAggregatesCounts(t *testing.T) {
	acc := newMetricAccumulator()
	acc.addFact(channelMonitorV2Fact{Success: 1})
	acc.addFact(channelMonitorV2Fact{Success: 1, Errors: 98})
	metric := acc.metric(2, false)
	require.Equal(t, int64(100), metric.RequestCount)
	require.InDelta(t, .02, metric.SuccessRate, 1e-9)

	acc.addHistogram(channelMonitorV2Histogram{Metric: "ttft", UpperBound: 100, Count: 1})
	acc.addHistogram(channelMonitorV2Histogram{Metric: "ttft", UpperBound: 1000, Count: 99})
	metric = acc.metric(2, false)
	require.NotNil(t, metric.TTFT.P90Ms)
	require.Equal(t, int64(1000), *metric.TTFT.P90Ms)
}
