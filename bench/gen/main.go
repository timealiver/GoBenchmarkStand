package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

var tagPool = []string{
	"error", "warn", "info", "debug",
	"http", "db", "cache", "queue",
	"auth", "payment", "search", "upload",
}

type event struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Value     float64  `json:"value"`
	Timestamp int64    `json:"timestamp"`
	Tags      []string `json:"tags"`
}

var namePool = []string{
	"cpu_usage", "memory_rss", "latency_p99", "request_rate",
	"error_rate", "cache_hit", "db_query_time", "queue_depth",
	"throughput", "active_connections",
}

func main() {
	outDir := flag.String("out", "bench/payloads", "output directory")
	seed := flag.Int64("seed", 42, "random seed for reproducibility")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}

	sizes := []struct {
		n    int
		name string
	}{
		{100, "payload_100.json"},
		{1_000, "payload_1k.json"},
		{10_000, "payload_10k.json"},
		{50_000, "payload_50k.json"},
	}

	rng := rand.New(rand.NewSource(*seed))
	now := time.Now().UnixMilli()

	for _, s := range sizes {
		events := make([]event, s.n)
		for i := range events {
			events[i] = randomEvent(rng, now, i)
		}

		path := filepath.Join(*outDir, s.name)
		f, err := os.Create(path)
		if err != nil {
			log.Fatalf("create %s: %v", path, err)
		}

		enc := json.NewEncoder(f)
		enc.SetIndent("", "")
		if err := enc.Encode(events); err != nil {
			f.Close()
			log.Fatalf("encode %s: %v", path, err)
		}
		f.Close()

		fi, _ := os.Stat(path)
		fmt.Printf("wrote %s  (%d events, %.1f KB)\n", path, s.n, float64(fi.Size())/1024)
	}
}

func randomEvent(rng *rand.Rand, baseTime int64, i int) event {
	numTags := rng.Intn(4) + 1
	tags := make([]string, numTags)
	for j := range tags {
		tags[j] = tagPool[rng.Intn(len(tagPool))]
	}

	return event{
		ID:        fmt.Sprintf("%016x%016x", rng.Uint64(), rng.Uint64()),
		Name:      namePool[rng.Intn(len(namePool))],
		Value:     rng.Float64() * 100,
		Timestamp: baseTime + int64(i)*10,
		Tags:      tags,
	}
}
