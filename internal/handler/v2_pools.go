package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"gostand/internal/aggregate"
	"gostand/internal/model"
)

// V2Pools reduces per-request allocations through three pool types:
//
//  1. bodyPool  — reuses *bytes.Buffer for reading the request body
//  2. eventPool — reuses *[]model.Event (reset to len=0 before decode)
//  3. valPool   — reuses *[]float64 for the aggregation sorting buffer
//
// json.Unmarshal is preferred over json.NewDecoder here because we already
// have the full body in a buffer, avoiding the decoder's internal buffering.
type V2Pools struct {
	bodyPool  sync.Pool
	eventPool sync.Pool
	valPool   sync.Pool
}

func NewV2Pools() *V2Pools {
	h := &V2Pools{}
	h.bodyPool.New = func() any {
		return bytes.NewBuffer(make([]byte, 0, 64*1024))
	}
	h.eventPool.New = func() any {
		s := make([]model.Event, 0, 512)
		return &s
	}
	h.valPool.New = func() any {
		s := make([]float64, 0, 512)
		return &s
	}
	return h
}

func (h *V2Pools) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Acquire and reset body buffer.
	buf := h.bodyPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer h.bodyPool.Put(buf)

	if _, err := io.Copy(buf, r.Body); err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	// Acquire and reset event slice.
	eventsPtr := h.eventPool.Get().(*[]model.Event)
	events := (*eventsPtr)[:0]
	defer func() {
		*eventsPtr = events[:0]
		h.eventPool.Put(eventsPtr)
	}()

	if err := json.Unmarshal(buf.Bytes(), &events); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Acquire and reset float buffer for aggregation.
	valPtr := h.valPool.Get().(*[]float64)
	vals := (*valPtr)[:0]

	result, vals := aggregate.StandardInto(events, defaultThreshold, vals)

	*valPtr = vals[:0]
	h.valPool.Put(valPtr)

	// Encode response into the reused body buffer.
	buf.Reset()
	if err := json.NewEncoder(buf).Encode(result); err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(buf.Bytes()) //nolint:errcheck
}
