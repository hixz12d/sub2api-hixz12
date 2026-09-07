package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorV2CardSettingsValidation(t *testing.T) {
	zero, warning, critical := int64(0), int64(1000), int64(5000)
	for _, tc := range []struct {
		name     string
		settings ChannelMonitorV2StatusCardSettings
		valid    bool
	}{
		{"unset", ChannelMonitorV2StatusCardSettings{}, true},
		{"valid", ChannelMonitorV2StatusCardSettings{&warning, &critical}, true},
		{"one missing", ChannelMonitorV2StatusCardSettings{&warning, nil}, false},
		{"zero", ChannelMonitorV2StatusCardSettings{&zero, &critical}, false},
		{"equal", ChannelMonitorV2StatusCardSettings{&warning, &warning}, false},
		{"reversed", ChannelMonitorV2StatusCardSettings{&critical, &warning}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := ChannelMonitorV2Config{StatusCardSettings: tc.settings}
			err := normalizeChannelMonitorV2Config(&cfg)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrChannelMonitorV2InvalidConfig)
			}
		})
	}
}

func TestChannelMonitorV2CardStatesUseIndependentMeasurements(t *testing.T) {
	warning, critical, p90 := int64(1000), int64(5000), int64(2000)
	m := ChannelMonitorV2Metric{RequestCount: 100, SuccessRequests: 90, ErrorRequests: 10, ErrorRate: 0, SuccessRate: .9, TTFT: ChannelMonitorV2Latency{SampleCount: 90, P90Ms: &p90}}
	require.Equal(t, "partial_failure", ChannelMonitorV2CardRequestState(m, DefaultChannelMonitorV2HealthThresholds()))
	require.Equal(t, "slow", ChannelMonitorV2CardPerformanceState(m, ChannelMonitorV2StatusCardSettings{&warning, &critical}, 50))
	require.Equal(t, "not_configured", ChannelMonitorV2CardPerformanceState(m, ChannelMonitorV2StatusCardSettings{}, 50))
	require.Equal(t, "no_data", ChannelMonitorV2CardRequestState(ChannelMonitorV2Metric{}, DefaultChannelMonitorV2HealthThresholds()))
	m.TTFT.SampleCount = 1
	require.Equal(t, "unknown", ChannelMonitorV2CardPerformanceState(m, ChannelMonitorV2StatusCardSettings{&warning, &critical}, 50))
}
