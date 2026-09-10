package service

import (
	"errors"
	"math"
	"sort"
)

// Bounds are milli tok/s, version 1. Each bucket stores an independent count.
// The final index len(bounds) is overflow and has no numeric percentile value.
var monitorTPSBoundsV1 = [...]int64{
	100, 105, 111, 117, 123, 130, 137, 144, 152, 160, 168, 177, 186, 196, 206, 217, 228, 240, 252, 265, 279, 293, 308, 324, 341, 359, 377, 396, 416, 437, 459, 482, 507, 533, 560, 588, 618, 649, 682, 717, 753, 791, 831, 873, 917, 963, 1012, 1063, 1117, 1173, 1232, 1294, 1359, 1427, 1499, 1574, 1653, 1736, 1823, 1915, 2011, 2112, 2218, 2329, 2446, 2569, 2698, 2833, 2975, 3124, 3281, 3446, 3619, 3800, 3990, 4190, 4400, 4620, 4851, 5094, 5349, 5617, 5898, 6193, 6503, 6829, 7171, 7530, 7907, 8303, 8719, 9155, 9613, 10094, 10599, 11129, 11686, 12271, 12885, 13530, 14207, 14918, 15664, 16448, 17271, 18135, 19042, 19995, 20995, 22045, 23148, 24306, 25522, 26799, 28139, 29546, 31024, 32576, 34205, 35916, 37712, 39598, 41578, 43657, 45840, 48132, 50539, 53066, 55720, 58506, 61432, 64504, 67730, 71117, 74673, 78407, 82328, 86445, 90768, 95307, 100073, 105077, 110331, 115848, 121641, 127724, 134111, 140817, 147858, 155251, 163014, 171165, 179724, 188711, 198147, 208055, 218458, 229381, 240851, 252894, 265539, 278816, 292757, 307395, 322765, 338904, 355850, 373643, 392326, 411943, 432541, 454169, 476878, 500722, 525759, 552047, 579650, 608633, 639065, 671019, 704570, 739799, 776789, 815629, 856411, 899232, 944194, 991404, 1040975, 1093024, 1147676, 1205060, 1265313, 1328579, 1395008, 1464759, 1537997, 1614897, 1695642, 1780425, 1869447, 1962920, 2061066, 2164120, 2272326, 2385943, 2505241, 2630504, 2762030, 2900132, 3045139, 3197396, 3357266, 3525130, 3701387, 3886457, 4080780, 4284819, 4499060, 4724013, 4960214, 5208225, 5468637, 5742069, 6029173, 6330632, 6647164, 6979523, 7328500, 7694925, 8079672, 8483656, 8907839, 9353231, 9820893, 10311938,
}

func MonitorTPSBoundsV1() []int64 {
	return append([]int64(nil), monitorTPSBoundsV1[:]...)
}

func MonitorTPSBucketV1(milli int64) (int, error) {
	if milli < 0 {
		return 0, errors.New("negative TPS")
	}
	return sort.Search(len(monitorTPSBoundsV1), func(i int) bool { return milli <= monitorTPSBoundsV1[i] }), nil
}

// MonitorTPSPercentileV1 returns an approximate upper bound in milli tok/s.
// Merge counts before calling; never average percentiles. Overflow is distinct
// from no data and from evidence strength, which the caller computes separately.
func MonitorTPSPercentileV1(counts []int64, percentile int) (*int64, string, error) {
	if len(counts) != len(monitorTPSBoundsV1)+1 || percentile < 1 || percentile > 100 {
		return nil, "", errors.New("invalid histogram")
	}
	var total int64
	for _, n := range counts {
		if n < 0 || n > math.MaxInt64-total {
			return nil, "", errors.New("invalid sample count")
		}
		total += n
	}
	if total == 0 {
		return nil, "no_data", nil
	}
	// Exact ceil(total*percentile/100) without multiplication overflow.
	rank := (total/100)*int64(percentile) + (total%100*int64(percentile)+99)/100
	var sum int64
	for i, n := range counts {
		sum += n
		if sum >= rank {
			if i == len(monitorTPSBoundsV1) {
				return nil, "out_of_range", nil
			}
			value := monitorTPSBoundsV1[i]
			return &value, "", nil
		}
	}
	return nil, "", errors.New("invalid histogram rank")
}
