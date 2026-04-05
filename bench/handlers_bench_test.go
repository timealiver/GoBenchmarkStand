package bench

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"gostand/internal/handler"
)

var payloadSizes = []struct {
	name string
	file string
}{
	{"100", "payloads/payload_100.json"},
	{"1k", "payloads/payload_1k.json"},
	{"10k", "payloads/payload_10k.json"},
	{"50k", "payloads/payload_50k.json"},
}

var handlers = []struct {
	name    string
	handler http.Handler
}{
	{"V1_Baseline", &handler.V1Baseline{}},
	{"V2_Pools", handler.NewV2Pools()},
	{"V3_Flat", handler.NewV3Flat()},
	{"V4_Sonic", handler.NewV4Sonic()},
}

func BenchmarkHandlers(b *testing.B) {
	for _, h := range handlers {
		h := h
		b.Run(h.name, func(b *testing.B) {
			for _, ps := range payloadSizes {
				ps := ps
				payload, err := os.ReadFile(ps.file)
				if err != nil {
					b.Skipf("payload file %s not found — run: go run ./bench/gen", ps.file)
				}

				b.Run(ps.name, func(b *testing.B) {
					b.ReportAllocs()
					b.SetBytes(int64(len(payload)))
					b.ResetTimer()

					for i := 0; i < b.N; i++ {
						req := httptest.NewRequest(http.MethodPost, "/aggregate",
							bytes.NewReader(payload))
						req.Header.Set("Content-Type", "application/json")

						rec := httptest.NewRecorder()
						h.handler.ServeHTTP(rec, req)

						if rec.Code != http.StatusOK {
							b.Fatalf("unexpected status %d", rec.Code)
						}
						io.Copy(io.Discard, rec.Body) //nolint:errcheck
					}
				})
			}
		})
	}
}

func BenchmarkAggregate_Isolated(b *testing.B) {
	for _, h := range handlers {
		h := h
		for _, ps := range payloadSizes {
			ps := ps
			payload, err := os.ReadFile(ps.file)
			if err != nil {
				continue
			}

			b.Run(h.name+"/"+ps.name, func(b *testing.B) {
				req := httptest.NewRequest(http.MethodPost, "/aggregate",
					bytes.NewReader(payload))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				h.handler.ServeHTTP(rec, req)

				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					req = httptest.NewRequest(http.MethodPost, "/aggregate",
						bytes.NewReader(payload))
					rec = httptest.NewRecorder()
					h.handler.ServeHTTP(rec, req)
				}
			})
		}
	}
}
