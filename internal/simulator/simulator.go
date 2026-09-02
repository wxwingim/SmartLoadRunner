// Package simulator реализует простого агента нагрузки: он генерит
// детерминированные метрики и POST-ит их координатору (как будущий runner).
package simulator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"time"

	"github.com/wxwingim/SmartLoadRunner/internal/models"
)

// Runner — агент-симулятор: раз в секунду генерит MetricBucket и POST-ит координатору.
type Runner struct {
	runID    string
	coordURL string // http://localhost:8080
	interval time.Duration
	vus      int
	duration time.Duration
	rate     int
	seed     int64
	client   *http.Client
}

// Opts — параметры создания симулятора.
type Opts struct {
	RunID    string
	CoordURL string
	Interval time.Duration
	VUs      int
	Duration time.Duration
	Rate     int
	Seed     int64
}

// New создаёт Runner из Opts с заполненными умолчаниями.
func New(o Opts) *Runner {
	if o.Interval <= 0 {
		o.Interval = time.Second
	}
	return &Runner{
		runID:    o.RunID,
		coordURL: o.CoordURL,
		interval: o.Interval,
		vus:      o.VUs,
		duration: o.Duration,
		rate:     o.Rate,
		seed:     o.Seed,
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

// Run — основной цикл: тикер → бакет → POST; завершение по ctx или длительности.
func (r *Runner) Run(ctx context.Context) error {
	start := time.Now()
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	// остановка по истечении длительности запуска
	timeout := time.After(r.duration)

	for {
		select {
		case <-ctx.Done():
			r.postFinal(start)
			return nil
		case <-timeout:
			r.postFinal(start)
			return nil
		case now := <-ticker.C:
			m := r.nextBucket(now.Sub(start))
			if err := r.post(ctx, m); err != nil {
				slog.Error("simulator: post metrics", "run", r.runID, "error", err)
			}
		}
	}
}

// postFinal отправляет последний бакет перед остановкой.
func (r *Runner) postFinal(start time.Time) {
	m := r.nextBucket(time.Since(start))
	if err := r.post(context.Background(), m); err != nil {
		slog.Error("simulator: post final metrics", "run", r.runID, "error", err)
	}
}

func (r *Runner) post(ctx context.Context, m *models.MetricBucket) error {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("simulator: marshal metric: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		r.coordURL+"/api/runs/"+m.RunID+"/metrics", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("simulator: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req) //nolint:gosec // адрес координатора из конфига, не пользовательский ввод
	if err != nil {
		return fmt.Errorf("simulator: post metrics: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Debug("simulator: close response body", "error", cerr)
		}
	}()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("simulator: coordinator: %s", resp.Status)
	}
	return nil
}

func (r *Runner) nextBucket(elapsed time.Duration) *models.MetricBucket {
	//nolint:gosec // детерминированный шум симуляции, не криптография
	rng := rand.New(rand.NewSource(r.seed + elapsed.Milliseconds()))
	// RPS: базовый + синусоидальная волна + небольшой шум
	rps := float64(r.rate) + 3*math.Sin(2*math.Pi*elapsed.Seconds()/20) + rng.Float64()*2
	p50 := 20 + rng.Float64()*40        // 20–60 ms
	p90 := p50 * 1.8                    // хвосты ~x1.8
	p99 := p50 * float64(3+rng.Intn(3)) // x3–x5 редкие выбросы
	return &models.MetricBucket{
		RunID:     r.runID,
		Timestamp: time.Now().UTC(),
		RPC:       rps,
		P50:       p50,
		P90:       p90,
		P99:       p99,
		Errors:    rng.Intn(3), // редкие ошибки
		ActiveVUs: r.vus,
	}
}
