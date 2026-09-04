// Package engine — оркестратор запусков: VU-воркеры, агрегация метрик,
// rate-limiter и HTTP-клиент runner↔координатор.
package engine

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/wxwingim/SmartLoadRunner/internal/models"
	"github.com/wxwingim/SmartLoadRunner/internal/scenario"
)

// Options — параметры одного прогона движка.
type Options struct {
	RunID    string
	Scenario *scenario.Scenario
	VUs      int
	Rate     int
	Seed     int64
	Timeout  time.Duration // таймаут одного HTTP-запроса
	Interval time.Duration // окно агрегации (рeпорт-интервал)
	Sink     func(m *models.MetricBucket) error
}

// Run исполняет сценарий, пока ctx не отменён или не истёк таймер длительности.
func Run(ctx context.Context, o Options) error {
	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Second
	}
	if o.Interval <= 0 {
		o.Interval = time.Second
	}

	agg := newAggregator()
	rl := newRateLimiter(o.Rate)

	client := &http.Client{Timeout: o.Timeout}

	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < o.VUs; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			w := &vuWorker{
				id: id, sc: o.Scenario, client: client,
				timeout: o.Timeout, obs: agg, rateLim: rl, ctx: ctx,
			}
			w.run()
		}(i)
	}

	// репортёр: раз в Interval снимаем окно и шлём в Sink
	t := time.NewTicker(o.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			// финальный бакет не снимаем: метрики уже отправлены Sink.
			wg.Wait()
			return nil
		case now := <-t.C:
			m := agg.snapshot(now.Sub(start), o.VUs)
			m.RunID = o.RunID
			if err := o.Sink(m); err != nil {
				slog.Error("engine: sink metrics", "run", o.RunID, "error", err)
			}
		}
	}
}
