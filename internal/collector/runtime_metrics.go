// Package collector provides background runtime metrics collection.
package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Snapshot is a subset of runtime.MemStats plus wall-clock time.
// Only the fields relevant for the benchmark report are included.
type Snapshot struct {
	Time        int64  `json:"time_unix_ms"`
	Version     string `json:"version"` // handler version label, e.g. "v1"
	NumGC       uint32 `json:"num_gc"`
	PauseNs     uint64 `json:"last_pause_ns"` // last GC stop-the-world pause
	HeapInuse   uint64 `json:"heap_inuse_bytes"`
	HeapObjects uint64 `json:"heap_objects"`
	Alloc       uint64 `json:"alloc_bytes"`       // currently allocated
	TotalAlloc  uint64 `json:"total_alloc_bytes"` // cumulative
	Sys         uint64 `json:"sys_bytes"`
	NumGoroutine int   `json:"num_goroutine"`
}

// Collector periodically samples runtime.MemStats and writes JSONL records to a file.
type Collector struct {
	version  string
	interval time.Duration
	path     string
}

// New creates a Collector that writes snapshots for the given handler version.
// outDir is the directory where the JSONL file is created.
func New(version, outDir string, interval time.Duration) *Collector {
	return &Collector{
		version:  version,
		interval: interval,
		path:     filepath.Join(outDir, fmt.Sprintf("metrics_%s.jsonl", version)),
	}
}

// Run starts collecting metrics until ctx is cancelled.
// It creates (or truncates) the output file and writes one JSON line per interval.
func (c *Collector) Run(ctx context.Context) error {
	f, err := os.Create(c.path)
	if err != nil {
		return fmt.Errorf("collector: create %s: %w", c.path, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			snap := c.sample()
			if err := enc.Encode(snap); err != nil {
				return fmt.Errorf("collector: encode: %w", err)
			}
		}
	}
}

func (c *Collector) sample() Snapshot {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	var lastPause uint64
	if ms.NumGC > 0 {
		// PauseNs is a circular buffer; the last pause is at index (NumGC+255)%256.
		lastPause = ms.PauseNs[(ms.NumGC+255)%256]
	}

	return Snapshot{
		Time:         time.Now().UnixMilli(),
		Version:      c.version,
		NumGC:        ms.NumGC,
		PauseNs:      lastPause,
		HeapInuse:    ms.HeapInuse,
		HeapObjects:  ms.HeapObjects,
		Alloc:        ms.Alloc,
		TotalAlloc:   ms.TotalAlloc,
		Sys:          ms.Sys,
		NumGoroutine: runtime.NumGoroutine(),
	}
}
