package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof/* routes on DefaultServeMux
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"gostand/internal/collector"
	"gostand/internal/handler"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	version := flag.String("version", "all", "handler version to collect metrics for (v1/v2/v3/v4/all)")
	outDir := flag.String("out", "results", "directory for metrics JSONL files")
	interval := flag.Duration("interval", 500*time.Millisecond, "metrics sampling interval")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("cannot create output dir: %v", err)
	}

	mux := http.NewServeMux()

	// Register the four handler versions.
	mux.Handle("/v1/aggregate", &handler.V1Baseline{})
	mux.Handle("/v2/aggregate", handler.NewV2Pools())
	mux.Handle("/v3/aggregate", handler.NewV3Flat())
	mux.Handle("/v4/aggregate", handler.NewV4Sonic())

	// /debug/metrics returns a snapshot of runtime.MemStats as JSON.
	mux.HandleFunc("/debug/metrics", metricsHandler)

	// Mount pprof routes from the default mux onto our mux.
	mux.Handle("/debug/pprof/", http.DefaultServeMux)

	srv := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start background metrics collector.
	col := collector.New(*version, *outDir, *interval)
	go func() {
		if err := col.Run(ctx); err != nil {
			log.Printf("collector error: %v", err)
		}
	}()

	go func() {
		log.Printf("gostand listening on %s", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down…")

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("graceful shutdown: %v", err)
	}
}

// metricsHandler returns a JSON snapshot of runtime.MemStats.
func metricsHandler(w http.ResponseWriter, _ *http.Request) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ms) //nolint:errcheck
}
