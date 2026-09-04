package engine

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/wxwingim/SmartLoadRunner/internal/scenario"
)

// resultObserver — интерфейс агрегатора результатов; реализуется Aggregator и моками в тестах.
type resultObserver interface {
	observe(latency time.Duration, ok bool)
}

// vuWorker выполняет сценарий одним виртуальным пользователем.
type vuWorker struct {
	id      int
	sc      *scenario.Scenario
	client  *http.Client
	timeout time.Duration
	obs     resultObserver
	rateLim *rateLimiter
	ctx     context.Context
}

func (v *vuWorker) run() {
	for {
		if err := v.rateLim.wait(v.ctx); err != nil {
			return // контекст отменён
		}
		for _, step := range v.sc.Steps {
			if err := v.ctx.Err(); err != nil {
				return
			}
			latency, ok := v.doStep(step)
			v.obs.observe(latency, ok)
			if step.ThinkTime > 0 {
				t := time.NewTimer(step.ThinkTime)
				select {
				case <-v.ctx.Done():
					t.Stop()
					return
				case <-t.C:
				}
			}
		}
	}
}

// doStep выполняет один шаг сценария и возвращает латентность и успех.
func (v *vuWorker) doStep(st scenario.Step) (time.Duration, bool) {
	ctx, cancel := context.WithTimeout(v.ctx, v.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, st.Method, st.URL, strings.NewReader(st.Body))
	if err != nil {
		return 0, false
	}
	for k, val := range st.Headers {
		req.Header.Set(k, val)
	}

	start := time.Now()
	resp, err := v.client.Do(req) //nolint:gosec // URL — из сценария пользователя, это и есть нагрузка
	lat := time.Since(start)
	if err != nil {
		return lat, false
	}
	// тело ответа нам не нужно, но close обязателен для повторного использования соединения
	_ = resp.Body.Close()
	// 4xx/5xx считаем ошибками сценария
	return lat, resp.StatusCode < 400
}
