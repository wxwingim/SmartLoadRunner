// Package httpapi содержит HTTP-хендлеры координатора: создание тестов,
// запуск run, приём метрик и SSE-стрим.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/wxwingim/SmartLoadRunner/internal/models"
	"github.com/wxwingim/SmartLoadRunner/internal/scenario"
	"github.com/wxwingim/SmartLoadRunner/internal/store"
)

// Handlers — HTTP-обработчики координатора.
type Handlers struct {
	Store       store.Storage
	Persistence *store.Persistence // файловый слой (может быть nil)
	IDGen       func() string
}

// CreateTest обрабатывает POST /api/tests.
func (h *Handlers) CreateTest(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body", err)
		return
	}

	s, err := scenario.Parse(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scenario", err)
		return
	}
	if err := s.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid scenario", err)
		return
	}

	t := &models.Test{
		ID:           h.IDGen(),
		Name:         s.Config.Name,
		ScenarioYaml: string(body),
		OwnerID:      "local", // спринт 1: без аутентификации
		CreatedAt:    time.Now().UTC(),
	}
	if err := h.Store.SaveTest(r.Context(), t); err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, "test already exists", err)
		} else {
			writeError(w, http.StatusInternalServerError, "save test", err)
		}
		return
	}
	if h.Persistence != nil {
		if err := h.Persistence.SaveTest(t); err != nil {
			writeError(w, http.StatusInternalServerError, "persist test", err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, t)
}

// StartRun обрабатывает POST /api/tests/{id}/run — создаёт и стартует run.
func (h *Handlers) StartRun(w http.ResponseWriter, r *http.Request) {
	testID := r.PathValue("id")

	t, err := h.Store.GetTest(r.Context(), testID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "test not found", err)
		} else {
			writeError(w, http.StatusInternalServerError, "get test", err)
		}
		return
	}

	s, err := scenario.Parse([]byte(t.ScenarioYaml))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "parse stored scenario", err)
		return
	}

	run := &models.Run{
		ID:          h.IDGen(),
		TestID:      testID,
		Status:      models.StateRunning,
		VUs:         s.Config.VUs,
		DurationSec: s.Config.DurationSec,
		Rate:        s.Config.Rate,
		Seed:        s.Config.Seed,
		StartAt:     time.Now().UTC(),
	}
	if err := h.Store.SaveRun(r.Context(), run); err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, "run already exists", err)
		} else {
			writeError(w, http.StatusInternalServerError, "save run", err)
		}
		return
	}
	if h.Persistence != nil {
		if err := h.Persistence.SaveRun(run); err != nil {
			writeError(w, http.StatusInternalServerError, "persist run", err)
			return
		}
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"run_id": run.ID})
}

// PostMetrics обрабатывает POST /api/runs/{id}/metrics — агент шлёт метрики.
func (h *Handlers) PostMetrics(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")

	var m models.MetricBucket
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeError(w, http.StatusBadRequest, "invalid metrics body", err)
		return
	}
	m.RunID = runID
	m.Timestamp = time.Now().UTC()

	if err := h.Store.AddMetrics(r.Context(), runID, &m); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "run not found", err)
		} else {
			writeError(w, http.StatusInternalServerError, "add metrics", err)
		}
		return
	}
	if h.Persistence != nil {
		if err := h.Persistence.AppendMetric(runID, &m); err != nil {
			writeError(w, http.StatusInternalServerError, "persist metrics", err)
			return
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

// StreamMetrics обрабатывает GET /api/runs/{id}/stream — SSE-стрим метрик.
func (h *Handlers) StreamMetrics(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")

	if _, err := h.Store.GetRun(r.Context(), runID); err != nil {
		writeError(w, http.StatusNotFound, "run not found", err)
		return
	}

	fl, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported", nil)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// отправляем заголовки сразу — клиент (curl/UI) должен увидеть установленный стрим
	// ещё до первого события; иначе он висит, пока не придёт первая метрика.
	fl.Flush()

	ch, cancel, err := h.Store.Subscribe(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "subscribe", err)
		return
	}
	defer cancel()

	// реплей уже накопленных метрик — клиент, подключившийся в середине, видит историю.
	if metrics, err := h.Store.GetMetrics(r.Context(), runID); err == nil {
		for _, m := range metrics {
			writeSSE(w, fl, m)
		}
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case m := <-ch:
			writeSSE(w, fl, m)
		}
	}
}

// Health обрабатывает GET /healthz — liveness-проба.
func (h *Handlers) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeSSE(w http.ResponseWriter, fl http.Flusher, m *models.MetricBucket) {
	payload, err := json.Marshal(m)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "marshal metrics", err)
		return
	}

	//nolint:gosec // payload — JSON нашего же энкодера, не пользовательский HTML
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return // клиент отвалился — флашить некуда
	}
	fl.Flush()
}
