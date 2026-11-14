package service

import (
	"sort"
	"time"
)

// resetCardAllocationEpsilon 用于浮点比较，低于该值的金额视为 0。
const resetCardAllocationEpsilon = 1e-9

// ResetCardAllocCredit 是分摊算法的输入：一笔限时额度。
type ResetCardAllocCredit struct {
	ID        int64
	GroupIDs  []int64
	Amount    float64
	StartsAt  time.Time
	ExpiresAt time.Time
}

// ResetCardSpendBucket 是分摊算法的输入：某个分组在某一小时内的消费合计。
// Hour 是该小时的起点（整点）。
type ResetCardSpendBucket struct {
	GroupID int64
	Hour    time.Time
	Amount  float64
}

// AllocateResetCardCredits 把消费按时间顺序分摊给限时额度，返回每笔额度已用掉的金额。
//
// 规则：
//   - 按时间顺序遍历消费桶（同一小时内按分组 ID 排序，保证结果确定）；
//   - 桶里的钱只分给"覆盖该分组、当时有效、还没用满"的额度；
//   - 多笔额度同时有效时，按 expires_at 从早到晚（相同再按 ID）依次填满；
//   - 同一笔消费只会分给一笔额度；每笔额度的已用金额不超过其 amount。
//
// "当时有效"按小时精度判断：桶覆盖的区间 [Hour, Hour+1h) 与额度的 [StartsAt, ExpiresAt) 有交集即视为有效。
// 由于是贪心按时间推进，一笔额度的结果只依赖于它到期之前的消费，到期后不会再变化。
func AllocateResetCardCredits(credits []ResetCardAllocCredit, buckets []ResetCardSpendBucket) map[int64]float64 {
	used := make(map[int64]float64, len(credits))
	if len(credits) == 0 {
		return used
	}

	ordered := make([]ResetCardAllocCredit, 0, len(credits))
	groupSets := make(map[int64]map[int64]struct{}, len(credits))
	for _, c := range credits {
		used[c.ID] = 0
		if c.Amount <= resetCardAllocationEpsilon {
			continue
		}
		set := make(map[int64]struct{}, len(c.GroupIDs))
		for _, g := range c.GroupIDs {
			set[g] = struct{}{}
		}
		groupSets[c.ID] = set
		ordered = append(ordered, c)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].ExpiresAt.Equal(ordered[j].ExpiresAt) {
			return ordered[i].ExpiresAt.Before(ordered[j].ExpiresAt)
		}
		return ordered[i].ID < ordered[j].ID
	})

	sortedBuckets := make([]ResetCardSpendBucket, 0, len(buckets))
	for _, b := range buckets {
		if b.Amount > resetCardAllocationEpsilon {
			sortedBuckets = append(sortedBuckets, b)
		}
	}
	sort.SliceStable(sortedBuckets, func(i, j int) bool {
		if !sortedBuckets[i].Hour.Equal(sortedBuckets[j].Hour) {
			return sortedBuckets[i].Hour.Before(sortedBuckets[j].Hour)
		}
		return sortedBuckets[i].GroupID < sortedBuckets[j].GroupID
	})

	for _, b := range sortedBuckets {
		remaining := b.Amount
		bucketEnd := b.Hour.Add(time.Hour)
		for _, c := range ordered {
			if remaining <= resetCardAllocationEpsilon {
				break
			}
			if _, ok := groupSets[c.ID][b.GroupID]; !ok {
				continue
			}
			// 区间 [Hour, Hour+1h) 与 [StartsAt, ExpiresAt) 有交集
			if !c.StartsAt.Before(bucketEnd) || !b.Hour.Before(c.ExpiresAt) {
				continue
			}
			room := c.Amount - used[c.ID]
			if room <= resetCardAllocationEpsilon {
				continue
			}
			take := remaining
			if take > room {
				take = room
			}
			used[c.ID] += take
			remaining -= take
		}
	}

	for id, v := range used {
		used[id] = roundResetCardAmount8(v)
	}
	return used
}

// resetCardAllocationScope 从某些目标额度出发，找出会影响它们分摊结果的全部额度，
// 并返回需要统计消费的时间起点。
//
// 两笔额度时间上有重叠时会争抢同一笔消费，影响会沿时间链条传递，
// 所以从目标额度最早的 starts_at 开始，把所有 expires_at 晚于该时间点的额度都纳入，
// 再用纳入额度里最早的 starts_at 重复，直到稳定。
func resetCardAllocationScope(all []ResetCardAllocCredit, targetIDs map[int64]struct{}) ([]ResetCardAllocCredit, time.Time, bool) {
	var lo time.Time
	found := false
	for _, c := range all {
		if _, ok := targetIDs[c.ID]; !ok {
			continue
		}
		if !found || c.StartsAt.Before(lo) {
			lo = c.StartsAt
			found = true
		}
	}
	if !found {
		return nil, time.Time{}, false
	}
	for {
		next := lo
		for _, c := range all {
			if c.ExpiresAt.After(lo) && c.StartsAt.Before(next) {
				next = c.StartsAt
			}
		}
		if next.Equal(lo) {
			break
		}
		lo = next
	}
	scope := make([]ResetCardAllocCredit, 0, len(all))
	for _, c := range all {
		if c.ExpiresAt.After(lo) {
			scope = append(scope, c)
		}
	}
	return scope, lo, true
}

func roundResetCardAmount8(v float64) float64 {
	if v < 0 {
		return 0
	}
	const scale = 1e8
	return float64(int64(v*scale+0.5)) / scale
}
