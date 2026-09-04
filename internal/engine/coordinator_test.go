package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testScenarioYAML = "config: {vus: 2, duration: 5, rate: 10, name: \"smoke\"}\nsteps: [{method: GET, url: https://example.com}]"

// configServer отдаёт JSON, идентичный GET /api/runs/{id}/config координатора.
func configServer(t *testing.T, scenarioYAML string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/runs/run-1/config" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"run_id":   "run-1",
			"scenario": scenarioYAML,
			"vus":      2,
			"duration": 5,
			"rate":     10,
			"seed":     42,
		})
	}))
}

func TestGetRunConfig(t *testing.T) {
	srv := configServer(t, testScenarioYAML)
	defer srv.Close()

	cfg, err := NewCoordinatorClient(srv.URL).GetRunConfig(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RunID != "run-1" || cfg.VUs != 2 || cfg.Rate != 10 || cfg.Seed != 42 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.Duration != 5*time.Second {
		t.Fatalf("duration = %v, want 5s", cfg.Duration)
	}
	if cfg.Scenario == nil {
		t.Fatal("scenario is nil")
	}
	if err := cfg.Scenario.Validate(); err != nil {
		t.Fatalf("parsed scenario invalid: %v", err)
	}
}

func TestGetRunConfigServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := NewCoordinatorClient(srv.URL).GetRunConfig(context.Background(), "no-such"); err == nil {
		t.Fatal("want error on 404 response")
	}
}

func TestGetRunConfigInvalidYAML(t *testing.T) {
	srv := configServer(t, "config: [broken")
	defer srv.Close()

	if _, err := NewCoordinatorClient(srv.URL).GetRunConfig(context.Background(), "run-1"); err == nil {
		t.Fatal("want error on unparseable scenario")
	}
}

func TestGetRunConfigFailsValidation(t *testing.T) {
	bad := "config: {vus: 0, duration: 5}\nsteps: [{method: GET, url: https://example.com}]"
	srv := configServer(t, bad)
	defer srv.Close()

	if _, err := NewCoordinatorClient(srv.URL).GetRunConfig(context.Background(), "run-1"); err == nil {
		t.Fatal("want error on scenario failing validation")
	}
}

func TestGetRunConfigRunIDMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"run_id":   "other-run",
			"scenario": testScenarioYAML,
			"vus":      2,
			"duration": 5,
			"rate":     10,
			"seed":     42,
		})
	}))
	defer srv.Close()

	if _, err := NewCoordinatorClient(srv.URL).GetRunConfig(context.Background(), "run-1"); err == nil {
		t.Fatal("want error on run_id mismatch")
	}
}
