package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorV2MeasurementBeforeRedaction(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int64
		state string
	}{
		{"empty", 0, "no_data"},
		{"low sample", 1, "low_sample"},
		{"sufficient", 50, "valid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := ChannelMonitorV2Metric{RequestCount: tc.count, ErrorRequests: tc.count, SuccessRate: 0}
			m.Measurement = ChannelMonitorV2MetricEvidence(m, 50)
			redactChannelMonitorV2Metric(&m, true)
			require.Equal(t, tc.state, m.Measurement.State)
			require.Equal(t, tc.count > 0, m.Measurement.HasRequests)
			require.Zero(t, m.RequestCount)
			require.Zero(t, m.SuccessRate)
			require.False(t, m.Measurement.HasCacheMeasurement)
			require.False(t, m.Measurement.HasTTFT)
		})
	}
}

func TestChannelMonitorV2MeasurementUsesAvailableSamples(t *testing.T) {
	m := ChannelMonitorV2Metric{RequestCount: 90, CacheRateDenominator: 10, TTFT: ChannelMonitorV2Latency{SampleCount: 1}, Duration: ChannelMonitorV2Latency{SampleCount: 2}}
	e := ChannelMonitorV2MetricEvidence(m, 50)
	require.True(t, e.HasTTFT)
	require.True(t, e.HasDuration)
	require.True(t, e.HasCacheMeasurement)
}
