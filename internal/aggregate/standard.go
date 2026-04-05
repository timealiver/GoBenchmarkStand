// Package aggregate provides aggregation logic used by all handler versions.
package aggregate

import (
	"math"
	"sort"

	"gostand/internal/model"
)

// Standard filters events where Value > threshold, then computes statistics.
// Used by V1 (baseline) and V2 (pools). Allocates a new []float64 for sorting.
func Standard(events []model.Event, threshold float64) model.AggregateResult {
	values := make([]float64, 0, len(events))
	sum := 0.0

	for i := range events {
		if events[i].Value > threshold {
			values = append(values, events[i].Value)
			sum += events[i].Value
		}
	}

	return buildResult(len(events), values, sum)
}

// StandardInto is the pool-friendly variant: it writes filtered values into the
// provided dst slice (reset to length 0 before call) to avoid an allocation.
func StandardInto(events []model.Event, threshold float64, dst []float64) (model.AggregateResult, []float64) {
	dst = dst[:0]
	sum := 0.0

	for i := range events {
		if events[i].Value > threshold {
			dst = append(dst, events[i].Value)
			sum += events[i].Value
		}
	}

	return buildResult(len(events), dst, sum), dst
}

func buildResult(total int, values []float64, sum float64) model.AggregateResult {
	n := len(values)
	if n == 0 {
		return model.AggregateResult{Count: total}
	}

	sort.Float64s(values)

	mean := sum / float64(n)

	return model.AggregateResult{
		Count:         total,
		FilteredCount: n,
		Sum:           math.Round(sum*1e6) / 1e6,
		Mean:          math.Round(mean*1e6) / 1e6,
		P50:           percentile(values, 0.50),
		P95:           percentile(values, 0.95),
		P99:           percentile(values, 0.99),
	}
}

// percentile computes the p-th percentile (0 < p <= 1) using linear interpolation.
func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 1 {
		return sorted[0]
	}
	rank := p * float64(n-1)
	lo := int(rank)
	hi := lo + 1
	if hi >= n {
		return sorted[n-1]
	}
	frac := rank - float64(lo)
	return sorted[lo] + frac*(sorted[hi]-sorted[lo])
}
