package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/wxwingim/SmartLoadRunner/internal/models"
	"github.com/wxwingim/SmartLoadRunner/internal/scenario"
)

// CoordinatorClient — HTTP-клиент агента к координатору.
type CoordinatorClient struct {
	baseURL string
	client  *http.Client
}

// NewCoordinatorClient создаёт HTTP-клиент агента к координатору.
func NewCoordinatorClient(baseURL string) *CoordinatorClient {
	return &CoordinatorClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// RegisterAgent отправляет POST /api/agents и возвращает назначенный agent_id.
func (c *CoordinatorClient) RegisterAgent(ctx context.Context, a *models.Agent) (string, error) {
	body, err := json.Marshal(a)
	if err != nil {
		return "", fmt.Errorf("marshal agent: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/agents", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build register request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req) //nolint:gosec // baseURL — из конфига
	if err != nil {
		return "", fmt.Errorf("register agent: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Debug("coordinator client: close body", "error", cerr)
		}
	}()
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("register agent: coordinator: %s", resp.Status)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode register response: %w", err)
	}
	return out.ID, nil
}

// RunConfig — «чистый» конфиг run для движка (без внутренних полей Run).
type RunConfig struct {
	RunID    string
	Scenario *scenario.Scenario
	VUs      int
	Duration time.Duration
	Rate     int
	Seed     int64
}

// GetRunConfig забирает GET /api/runs/{id}/config, парсит сценарий и собирает RunConfig для движка.
func (c *CoordinatorClient) GetRunConfig(ctx context.Context, runID string) (*RunConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/runs/"+runID+"/config", nil)
	if err != nil {
		return nil, fmt.Errorf("get run config: build request: %w", err)
	}
	resp, err := c.client.Do(req) //nolint:gosec // адрес координатора из конфига
	if err != nil {
		return nil, fmt.Errorf("get run config: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Debug("coordinator client: close body", "error", cerr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get run config: coordinator: %s", resp.Status)
	}

	var dto struct {
		RunID    string `json:"run_id"`
		Scenario string `json:"scenario"`
		VUs      int    `json:"vus"`
		Duration int    `json:"duration"` // секунды
		Rate     int    `json:"rate"`
		Seed     int64  `json:"seed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		return nil, fmt.Errorf("get run config: decode response: %w", err)
	}
	if dto.RunID != runID {
		return nil, fmt.Errorf("get run config: coordinator returned run_id %q, want %q", dto.RunID, runID)
	}

	sc, err := scenario.Parse([]byte(dto.Scenario))
	if err != nil {
		return nil, fmt.Errorf("get run config: parse scenario: %w", err)
	}
	if err := sc.Validate(); err != nil {
		return nil, fmt.Errorf("get run config: validate scenario: %w", err)
	}

	return &RunConfig{
		RunID:    dto.RunID,
		Scenario: sc,
		VUs:      dto.VUs,
		Duration: time.Duration(dto.Duration) * time.Second,
		Rate:     dto.Rate,
		Seed:     dto.Seed,
	}, nil
}

// ReportMetrics отправляет POST /api/runs/{id}/metrics.
func (c *CoordinatorClient) ReportMetrics(ctx context.Context, m *models.MetricBucket) error {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("simulator: marshal metric: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/runs/"+m.RunID+"/metrics", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("simulator: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req) //nolint:gosec // адрес координатора из конфига, не пользовательский ввод
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
