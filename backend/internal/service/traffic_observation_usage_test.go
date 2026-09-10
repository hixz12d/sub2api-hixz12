package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCaptureMonitorUsageSnapshot(t *testing.T) {
	start := time.Unix(100, 0)
	first, last := start.Add(time.Second), start.Add(3*time.Second)
	input, cached, visible := int64(100), int64(0), int64(80)
	observation := &VisibleOutputObservation{
		Origin: RequestOriginBusiness, RequestStartedAt: start,
		FirstVisibleTextAt: &first, LastVisibleTextAt: &last,
		InputTokensTotal: &input, CacheReadTokens: &cached, VisibleOutputTokens: &visible,
		OutputComplete: true, TextDeltaCount: 2,
		Version: VisibleStreamObservationVersion, Method: VisibleStreamObservationMethod,
	}
	fields := CaptureMonitorUsage(observation)
	require.Equal(t, int64(0), *fields.MonitorCacheReadTokens)
	require.Equal(t, int64(100), *fields.MonitorInputTokensTotal)
	require.Equal(t, int64(80), *fields.MonitorVisibleOutputTokens)
	require.Equal(t, int64(2000), *fields.MonitorGenerationMs)
	require.Equal(t, int64(40000), *fields.MonitorOutputTPSMilli)
	require.Equal(t, int64(1000), *fields.MonitorFirstVisibleMs)
	input, cached, visible = 999, 999, 999
	require.Equal(t, int64(100), *fields.MonitorInputTokensTotal)
	require.Equal(t, int64(0), *fields.MonitorCacheReadTokens)
	require.Equal(t, int64(80), *fields.MonitorVisibleOutputTokens)
	require.Zero(t, fields.InputTokens)
	require.Zero(t, fields.OutputTokens)
	require.Zero(t, fields.TotalCost)
	require.Zero(t, fields.ActualCost)
}

func TestCaptureMonitorUsageMissingAndInvalidSamples(t *testing.T) {
	require.Equal(t, UsageLog{}, CaptureMonitorUsage(nil))
	require.Equal(t, UsageLog{}, CaptureMonitorUsage(&VisibleOutputObservation{Origin: "untrusted"}))
	for _, tc := range []struct {
		name     string
		cached   *int64
		complete bool
	}{
		{"missing cache", nil, true},
		{"negative cache", monitorTestInt64(-1), true},
		{"cache exceeds input", monitorTestInt64(101), true},
		{"incomplete output", monitorTestInt64(0), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Unix(100, 0)
			first, last := start.Add(time.Second), start.Add(3*time.Second)
			o := &VisibleOutputObservation{Origin: RequestOriginBusiness, Version: VisibleStreamObservationVersion, Method: VisibleStreamObservationMethod, InputTokensTotal: monitorTestInt64(100), CacheReadTokens: tc.cached, RequestStartedAt: start, FirstVisibleTextAt: &first, LastVisibleTextAt: &last, OutputComplete: tc.complete}
			fields := CaptureMonitorUsage(o)
			require.Nil(t, fields.MonitorOutputTPSMilli)
			require.Nil(t, fields.MonitorVisibleOutputTokens)
			require.Nil(t, fields.MonitorTPSMethod)
			if tc.name != "incomplete output" {
				require.Nil(t, fields.MonitorCacheReadTokens)
			} else {
				require.Equal(t, int64(0), *fields.MonitorCacheReadTokens)
				require.Nil(t, fields.MonitorFirstVisibleMs)
			}
		})
	}
	o := &VisibleOutputObservation{Origin: RequestOriginBusiness, Version: VisibleStreamObservationVersion + 1, Method: VisibleStreamObservationMethod, InputTokensTotal: monitorTestInt64(100)}
	fields := CaptureMonitorUsage(o)
	require.NotNil(t, fields.RequestOrigin)
	require.Nil(t, fields.MonitorObservationVersion)
	require.Nil(t, fields.MonitorInputTokensTotal)
}

func monitorTestInt64(value int64) *int64 { return &value }
