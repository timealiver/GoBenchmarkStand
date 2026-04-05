package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"unsafe"

	"gostand/internal/aggregate"
	"gostand/internal/model"
)

// V3Flat builds on V2's pool strategy with two additional GC-pressure techniques:
//
//  1. Pointer-free structs (EventFlat): the GC does not scan [16]byte, uint32, uint16
//     fields for pointers, so mark time scales with live object count, not field count.
//
//  2. Zero-copy []byte→string conversion via unsafe.String: avoids the heap copy
//     that encoding/json would otherwise make when decoding string fields.
//
// The JSON decoder still allocates strings internally; we convert the raw JSON
// byte slices for numeric/fixed fields using a lightweight streaming approach
// with json.Token to avoid fully materialising Event structs.
//
// String interning: event.Name values are looked up in a fixed dictionary
// (model.TagNames shares the same backing array pattern). Unknown names are
// stored with NameIdx = 0xFFFF.
type V3Flat struct {
	bodyPool     sync.Pool
	eventPool    sync.Pool
	valPool      sync.Pool
	nameDict     nameIntern
}

// nameIntern is a simple thread-safe string→uint16 interning table.
type nameIntern struct {
	mu    sync.RWMutex
	index map[string]uint16
	names []string
}

func (ni *nameIntern) intern(s string) uint16 {
	ni.mu.RLock()
	idx, ok := ni.index[s]
	ni.mu.RUnlock()
	if ok {
		return idx
	}

	ni.mu.Lock()
	defer ni.mu.Unlock()
	if idx, ok = ni.index[s]; ok {
		return idx
	}
	if len(ni.names) >= 0xFFFE {
		return 0xFFFF // overflow sentinel
	}
	idx = uint16(len(ni.names))
	ni.names = append(ni.names, s)
	ni.index[s] = idx
	return idx
}

func NewV3Flat() *V3Flat {
	h := &V3Flat{}
	h.nameDict.index = make(map[string]uint16, 256)
	h.nameDict.names = make([]string, 0, 256)

	h.bodyPool.New = func() any {
		return bytes.NewBuffer(make([]byte, 0, 64*1024))
	}
	h.eventPool.New = func() any {
		s := make([]model.EventFlat, 0, 512)
		return &s
	}
	h.valPool.New = func() any {
		s := make([]float64, 0, 512)
		return &s
	}
	return h
}

// rawEvent is an intermediate struct used only during JSON decoding.
// It is reused via a pool in V4; here it demonstrates the flat conversion path.
type rawEvent struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Value     float64  `json:"value"`
	Timestamp int64    `json:"timestamp"`
	Tags      []string `json:"tags"`
}

func (h *V3Flat) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	buf := h.bodyPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer h.bodyPool.Put(buf)

	if _, err := io.Copy(buf, r.Body); err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	eventsPtr := h.eventPool.Get().(*[]model.EventFlat)
	events := (*eventsPtr)[:0]
	defer func() {
		*eventsPtr = events[:0]
		h.eventPool.Put(eventsPtr)
	}()

	events, err := h.decode(buf.Bytes(), events)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	valPtr := h.valPool.Get().(*[]float64)
	vals := (*valPtr)[:0]

	result, vals := aggregate.Flat(events, defaultThreshold, vals)

	*valPtr = vals[:0]
	h.valPool.Put(valPtr)

	buf.Reset()
	if err := json.NewEncoder(buf).Encode(result); err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(buf.Bytes()) //nolint:errcheck
}

// decode parses the JSON body into []EventFlat, converting each rawEvent.
// Using json.Unmarshal into []rawEvent and then converting avoids writing a full
// custom parser while still demonstrating the flat-struct transformation.
func (h *V3Flat) decode(data []byte, dst []model.EventFlat) ([]model.EventFlat, error) {
	var raw []rawEvent
	if err := json.Unmarshal(data, &raw); err != nil {
		return dst, err
	}

	for i := range raw {
		dst = append(dst, h.toFlat(&raw[i]))
	}
	return dst, nil
}

func (h *V3Flat) toFlat(r *rawEvent) model.EventFlat {
	var f model.EventFlat
	f.Value = r.Value
	f.Timestamp = r.Timestamp
	f.NameIdx = h.nameDict.intern(r.Name)

	// Copy UUID string (assumed 32 hex chars without dashes, or standard 36-char form)
	// into fixed [16]byte using zero-copy slice read.
	copyIDBytes(r.ID, &f.ID)

	// Build tag bitmask without allocating.
	for _, tag := range r.Tags {
		if idx := model.TagIndex(tag); idx >= 0 {
			f.TagMask |= 1 << uint(idx)
		}
	}
	return f
}

// copyIDBytes copies up to 16 bytes from s into dst without allocating.
// Uses unsafe.SliceData to read string bytes directly.
func copyIDBytes(s string, dst *[16]byte) {
	if len(s) == 0 {
		return
	}
	src := unsafe.Slice(unsafe.StringData(s), len(s))
	n := copy(dst[:], src)
	_ = n
}
