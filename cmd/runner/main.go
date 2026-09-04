package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wxwingim/SmartLoadRunner/internal/config"
	"github.com/wxwingim/SmartLoadRunner/internal/engine"
	"github.com/wxwingim/SmartLoadRunner/internal/models"
)

// version задаётся при сборке: -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	runID := flag.String("run", "", "run id to execute")
	flag.Parse()
	if *runID == "" {
		slog.Error("flag -run is required")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := engine.NewCoordinatorClient(cfg.Runner.CoordinatorAddr)

	agent := &models.Agent{Version: version, Capacity: cfg.Runner.Capacity}
	agentID, err := client.RegisterAgent(ctx, agent)
	if err != nil {
		slog.Error("register agent", "error", err)
		os.Exit(1)
	}
	slog.Info("agent registered", "agent_id", agentID)

	runCfg, err := client.GetRunConfig(ctx, *runID)
	if err != nil {
		slog.Error("fetch run config", "run", *runID, "error", err)
		os.Exit(1)
	}
	// таймер ограничивает длительность; Ctrl+C останавливает run через отмену контекста
	runCtx, cancel := context.WithCancel(ctx)
	time.AfterFunc(runCfg.Duration, cancel)

	if err := engine.Run(runCtx, engine.Options{
		RunID:    runCfg.RunID,
		Scenario: runCfg.Scenario,
		VUs:      runCfg.VUs,
		Rate:     runCfg.Rate,
		Seed:     runCfg.Seed,
		Timeout:  time.Duration(cfg.Runner.RequestTimeoutSec) * time.Second,
		Interval: time.Duration(cfg.Runner.ReportIntervalSec) * time.Second,
		Sink: func(m *models.MetricBucket) error {
			return client.ReportMetrics(context.Background(), m)
		},
	}); err != nil {
		slog.Error("run failed", "run", *runID, "error", err)
		os.Exit(1)
	}
	slog.Info("run finished", "run", *runID, "agent", agentID)
}
