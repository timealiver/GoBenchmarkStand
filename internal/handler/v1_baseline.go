// Package handler contains the four handler implementations for the benchmark stand.
package handler

import (
	"encoding/json"
	"net/http"

	"gostand/internal/aggregate"
	"gostand/internal/model"
)

const defaultThreshold = 50.0

// V1Baseline is the canonical Go handler with no manual memory optimizations.
//
// Characteristics:
//   - json.NewDecoder allocates an internal buffer per call
//   - Decode(&events) allocates a new []Event and each Event's string/slice fields
//   - aggregate.Standard allocates a []float64 for sorting
//   - json.NewEncoder + Encode allocates an intermediate buffer per call
//
// This serves as the measurement baseline.
type V1Baseline struct{}

func (h *V1Baseline) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var events []model.Event
	if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	result := aggregate.Standard(events, defaultThreshold)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
	}
}
