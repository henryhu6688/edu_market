package main

import "sort"

// precisionAtK = |前K条 ∩ ground_truth| / K
func precisionAtK(returned, relevant []uint, k int) float64 {
	if k == 0 {
		return 0
	}
	top := returned
	if len(top) > k {
		top = top[:k]
	}
	return float64(intersectCount(top, toSet(relevant))) / float64(k)
}

// recallAtK = |前K条 ∩ ground_truth| / |ground_truth|
func recallAtK(returned, relevant []uint, k int) float64 {
	if len(relevant) == 0 {
		return 0
	}
	top := returned
	if len(top) > k {
		top = top[:k]
	}
	return float64(intersectCount(top, toSet(relevant))) / float64(len(relevant))
}

// topKHit = 前K条是否至少命中一条 ground truth
func topKHit(returned, relevant []uint, k int) bool {
	set := toSet(relevant)
	limit := k
	if len(returned) < limit {
		limit = len(returned)
	}
	for _, id := range returned[:limit] {
		if set[id] {
			return true
		}
	}
	return false
}

// toSet 将切片转为 set
func toSet(ids []uint) map[uint]bool {
	s := make(map[uint]bool, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}

// intersectCount 返回 ids 中有多少个在 set 里
func intersectCount(ids []uint, set map[uint]bool) int {
	n := 0
	for _, id := range ids {
		if set[id] {
			n++
		}
	}
	return n
}

// latencyPercentiles 从延迟列表计算分位数（需先排序）
func latencyPercentiles(sortedMs []float64) (avg, p50, p95, p99 float64) {
	if len(sortedMs) == 0 {
		return 0, 0, 0, 0
	}
	sum := 0.0
	for _, v := range sortedMs {
		sum += v
	}
	avg = sum / float64(len(sortedMs))

	idx50 := int(float64(len(sortedMs)) * 0.50)
	idx95 := int(float64(len(sortedMs)) * 0.95)
	idx99 := int(float64(len(sortedMs)) * 0.99)
	if idx50 >= len(sortedMs) {
		idx50 = len(sortedMs) - 1
	}
	if idx95 >= len(sortedMs) {
		idx95 = len(sortedMs) - 1
	}
	if idx99 >= len(sortedMs) {
		idx99 = len(sortedMs) - 1
	}

	return avg, sortedMs[idx50], sortedMs[idx95], sortedMs[idx99]
}

// configSummary 从一组 RAGQueryResult 汇总指标
func configSummary(cfgName string, results []RAGQueryResult) ConfigSummary {
	s := ConfigSummary{ConfigName: cfgName}
	if len(results) == 0 {
		return s
	}
	var sumP, sumR float64
	hits := 0
	var lats []float64
	for _, r := range results {
		sumP += r.PrecisionAtK
		sumR += r.RecallAtK
		if r.TopKHit {
			hits++
		}
		lats = append(lats, r.LatencyMs)
	}
	s.AvgPrecision = sumP / float64(len(results))
	s.AvgRecall = sumR / float64(len(results))
	s.TopKHitRate = float64(hits) / float64(len(results))

	sort.Float64s(lats)
	avg, _, _, _ := latencyPercentiles(lats)
	s.AvgLatencyMs = avg
	return s
}
