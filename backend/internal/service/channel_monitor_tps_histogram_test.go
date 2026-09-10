package service

import (
	"github.com/stretchr/testify/require"
	"math"
	"testing"
)

func TestMonitorTPSHistogramV1(t *testing.T) {
	bounds := MonitorTPSBoundsV1()
	require.Len(t, bounds, 236)
	require.Equal(t, int64(100), bounds[0])
	require.Equal(t, int64(10311938), bounds[len(bounds)-1])
	for i := 1; i < len(bounds); i++ {
		require.Equal(t, (bounds[i-1]*105+99)/100, bounds[i])
	}
	for i, v := range bounds {
		idx, err := MonitorTPSBucketV1(v)
		require.NoError(t, err)
		require.Equal(t, i, idx)
	}
	bounds[0] = 999
	require.Equal(t, int64(100), MonitorTPSBoundsV1()[0])
	counts := make([]int64, 237)
	value, reason, err := MonitorTPSPercentileV1(counts, 50)
	require.NoError(t, err)
	require.Nil(t, value)
	require.Equal(t, "no_data", reason)
	counts[0], counts[1], counts[236] = 1, 2, 1
	value, reason, err = MonitorTPSPercentileV1(counts, 50)
	require.NoError(t, err)
	require.Empty(t, reason)
	require.Equal(t, int64(105), *value)
	value, reason, err = MonitorTPSPercentileV1(counts, 100)
	require.NoError(t, err)
	require.Nil(t, value)
	require.Equal(t, "out_of_range", reason)
	counts[0] = math.MaxInt64
	_, _, err = MonitorTPSPercentileV1(counts, 50)
	require.Error(t, err)
	counts = make([]int64, 237)
	counts[0] = math.MaxInt64
	value, _, err = MonitorTPSPercentileV1(counts, 99)
	require.NoError(t, err)
	require.Equal(t, int64(100), *value)
	_, _, err = MonitorTPSPercentileV1(counts, 0)
	require.Error(t, err)
	_, err = MonitorTPSBucketV1(-1)
	require.Error(t, err)
}
