package aggregate

import (
	"math"
	"sort"

	"gostand/internal/model"
)

// Flat filters EventFlat slice where Value > threshold and computes statistics.
// Used by V3 and V4. Works on pointer-free structs; dst avoids extra allocation.
func Flat(events []model.EventFlat, threshold float64, dst []float64) (model.AggregateResult, []float64) {
	dst = dst[:0]
	sum := 0.0

	for i := range events {
		if events[i].Value > threshold {
			dst = append(dst, events[i].Value)
			sum += events[i].Value
		}
	}

	n := len(dst)
	if n == 0 {
		return model.AggregateResult{Count: len(events)}, dst
	}

	sort.Float64s(dst)

	mean := sum / float64(n)

	result := model.AggregateResult{
		Count:         len(events),
		FilteredCount: n,
		Sum:           math.Round(sum*1e6) / 1e6,
		Mean:          math.Round(mean*1e6) / 1e6,
		P50:           percentile(dst, 0.50),
		P95:           percentile(dst, 0.95),
		P99:           percentile(dst, 0.99),
	}

	return result, dst
}
