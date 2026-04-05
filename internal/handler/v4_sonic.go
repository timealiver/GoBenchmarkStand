package handler

import (
	"bytes"
	"io"
	"net/http"
	"sync"
	"unsafe"

	"github.com/bytedance/sonic"

	"gostand/internal/aggregate"
	"gostand/internal/model"
)

// V4Sonic replaces encoding/json with bytedance/sonic for both decode and encode.
//
// Sonic uses SIMD-accelerated JSON parsing on AMD64/ARM64 Linux. On other
// platforms (including Windows AMD64) it falls back to a pure-Go compatible
// mode that still outperforms encoding/json due to a faster reflection engine.
//
// Combined with the V3 flat-struct approach this version minimises both:
//   - JSON serialisation cost (sonic vs stdlib)
//   - GC scan cost (pointer-free EventFlat)
//
// NOTE: run load tests on Linux/AMD64 to measure the full SIMD benefit.
// Document the GOOS/GOARCH and sonic.ConfigDefault.Pretouch() in the report.
type V4Sonic struct {
	bodyPool  sync.Pool
	eventPool sync.Pool
	valPool   sync.Pool
	nameDict  nameIntern
	api       sonic.API
}

func NewV4Sonic() *V4Sonic {
	h := &V4Sonic{}
	h.nameDict.index = make(map[string]uint16, 256)
	h.nameDict.names = make([]string, 0, 256)
	h.api = sonic.ConfigDefault

	h.bodyPool.New = func() any {
		return bytes.NewBuffer(make([]byte, 0, 64*1024))
	}
	h.eventPool.New = func() any {
		s := make([]rawEvent, 0, 512)
		return &s
	}
	h.valPool.New = func() any {
		s := make([]float64, 0, 512)
		return &s
	}
	return h
}

func (h *V4Sonic) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	buf := h.bodyPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer h.bodyPool.Put(buf)

	if _, err := io.Copy(buf, r.Body); err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	rawPtr := h.eventPool.Get().(*[]rawEvent)
	raw := (*rawPtr)[:0]
	defer func() {
		*rawPtr = raw[:0]
		h.eventPool.Put(rawPtr)
	}()

	if err := h.api.Unmarshal(buf.Bytes(), &raw); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Convert []rawEvent → []EventFlat on the stack without extra allocation.
	// We size an EventFlat slice on-demand; it is not pooled here because
	// EventFlat is a value type and the GC cost is negligible.
	flatEvents := make([]model.EventFlat, len(raw))
	for i := range raw {
		flatEvents[i] = h.toFlatV4(&raw[i])
	}

	valPtr := h.valPool.Get().(*[]float64)
	vals := (*valPtr)[:0]

	result, vals := aggregate.Flat(flatEvents, defaultThreshold, vals)

	*valPtr = vals[:0]
	h.valPool.Put(valPtr)

	encoded, err := h.api.Marshal(result)
	if err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(encoded) //nolint:errcheck
}

func (h *V4Sonic) toFlatV4(r *rawEvent) model.EventFlat {
	var f model.EventFlat
	f.Value = r.Value
	f.Timestamp = r.Timestamp
	f.NameIdx = h.nameDict.intern(r.Name)

	copyIDBytes4(r.ID, &f.ID)

	for _, tag := range r.Tags {
		if idx := model.TagIndex(tag); idx >= 0 {
			f.TagMask |= 1 << uint(idx)
		}
	}
	return f
}

func copyIDBytes4(s string, dst *[16]byte) {
	if len(s) == 0 {
		return
	}
	src := unsafe.Slice(unsafe.StringData(s), len(s))
	copy(dst[:], src)
}
