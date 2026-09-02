// принимает сценарий YAML, стартует run (здесь — fake metrics simulator)
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wxwingim/SmartLoadRunner/httpapi"
	"github.com/wxwingim/SmartLoadRunner/internal/config"
	"github.com/wxwingim/SmartLoadRunner/internal/idgen"
	"github.com/wxwingim/SmartLoadRunner/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st := store.NewInMemoryStore()
	persist, err := store.NewPersistence("data/tests", "data/runs")
	if err != nil {
		slog.Error("init persistence", "error", err)
		os.Exit(1)
	}
	if err := persist.LoadAll(ctx, st); err != nil {
		slog.Warn("load metadata", "error", err)
	}

	h := &httpapi.Handlers{Store: st, Persistence: persist, IDGen: idgen.New}
	srv := &http.Server{
		Addr:        cfg.Addr(),
		Handler:     httpapi.NewRouter(h),
		ReadTimeout: time.Duration(cfg.HTTP.ReadTimeout) * time.Second,
		IdleTimeout: time.Duration(cfg.HTTP.IdleTimeout) * time.Second,
		// WriteTimeout: 0 — см. камень про SSE
	}

	go func() {
		slog.Info("coordinator listening", "addr", cfg.Addr())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	slog.Info("coordinator stopped")
}
