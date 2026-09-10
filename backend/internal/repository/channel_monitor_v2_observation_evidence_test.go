package repository

import (
	"github.com/stretchr/testify/require"
	"math"
	"testing"
)

func TestChannelMonitorObservationSampleState(t *testing.T) {
	for _, tc := range []struct {
		name    string
		counts  []int64
		minimum int64
		want    string
	}{
		{"empty", nil, 50, "no_data"},
		{"below", []int64{20, 29}, 50, "low_sample"},
		{"boundary", []int64{20, 30}, 50, "valid"},
		{"default", []int64{49}, 0, "low_sample"},
		{"large", []int64{math.MaxInt64, math.MaxInt64}, 50, "valid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, channelMonitorObservationSampleState(tc.counts, tc.minimum))
		})
	}
}
