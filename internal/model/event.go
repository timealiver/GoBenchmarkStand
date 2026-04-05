package model

// Event represents a single metric event (V1, V2).
// Uses standard Go types with pointer-based fields (string, []string).
type Event struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Value     float64  `json:"value"`
	Timestamp int64    `json:"timestamp"`
	Tags      []string `json:"tags"`
}

// AggregateResult is the response returned by all four endpoints.
type AggregateResult struct {
	Count         int     `json:"count"`
	FilteredCount int     `json:"filtered_count"`
	Sum           float64 `json:"sum"`
	Mean          float64 `json:"mean"`
	P50           float64 `json:"p50"`
	P95           float64 `json:"p95"`
	P99           float64 `json:"p99"`
}
