// принимает сценарий file path or stdin + flags (vus, duration, rate) и стартует VU goroutines;
// каждая VU выполняет steps sequentially.
// Аггрегатор кажду секунду рассчитывает RPS и percentiles (for MVP p50/p90 via sorting small slice).
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wxwingim/SmartLoadRunner/internal/simulator"
)

func main() {
	runID := flag.String("run", "", "run id to simulate")
	coord := flag.String("coord", "http://localhost:8080", "coordinator base url")
	interval := flag.Int("interval", 1, "report interval, seconds")
	vus := flag.Int("vus", 5, "active VUs")
	duration := flag.Int("duration", 30, "run duration, seconds")
	rate := flag.Int("rate", 10, "target RPS")
	seed := flag.Int64("seed", 1, "reproducible seed")
	flag.Parse()
	if *runID == "" {
		slog.Error("flag -run is required")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	r := simulator.New(simulator.Opts{
		RunID: *runID, CoordURL: *coord, Interval: time.Duration(*interval) * time.Second,
		VUs: *vus, Duration: time.Duration(*duration) * time.Second,
		Rate: *rate, Seed: *seed,
	})
	if err := r.Run(ctx); err != nil {
		slog.Error("simulation failed", "error", err)
		os.Exit(1)
	}
}
