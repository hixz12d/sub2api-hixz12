package repository

import "github.com/Wei-Shaw/sub2api/internal/service"

// Counts are capped at the configured threshold: no raw volume is exposed and
// summing large disjoint histogram buckets cannot overflow.
func channelMonitorObservationSampleState(counts []int64, minimum int64) string {
	if minimum <= 0 {
		minimum = service.DefaultChannelMonitorV2HealthThresholds().MinimumSample
	}
	var total int64
	for _, count := range counts {
		if count <= 0 {
			continue
		}
		if count >= minimum-total {
			return "valid"
		}
		total += count
	}
	if total == 0 {
		return "no_data"
	}
	return "low_sample"
}
