package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wxwingim/SmartLoadRunner/internal/models"
	"github.com/wxwingim/SmartLoadRunner/internal/store"
)

func setup(t *testing.T) (*httptest.Server, *store.InMemoryStore) {
	t.Helper()
	st := store.NewInMemoryStore()
	var n int
	h := &Handlers{Store: st, IDGen: func() string { n++; return fmt.Sprintf("id-%d", n) }}
	srv := httptest.NewServer(NewRouter(h))
	t.Cleanup(srv.Close)
	return srv, st
}

func TestCreateTest(t *testing.T) {
	srv, _ := setup(t)
	body := `config: {vus: 2, duration: 5, rate: 10, name: "smoke"}
steps: [{method: GET, url: https://example.com}]`

	resp, err := http.Post(srv.URL+"/api/tests", "application/yaml", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Logf("close response body: %v", cerr)
		}
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create test: status=%d", resp.StatusCode)
	}
	var test models.Test
	if err := json.NewDecoder(resp.Body).Decode(&test); err != nil {
		t.Fatal(err)
	}
	if test.ID == "" || test.Name != "smoke" {
		t.Fatalf("unexpected test: %+v", test)
	}
}

func TestCreateTestInvalidYAML(t *testing.T) {
	srv, _ := setup(t)
	resp, err := http.Post(srv.URL+"/api/tests", "application/yaml", strings.NewReader("config: [bad"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Logf("close response body: %v", cerr)
		}
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestStreamReceivesMetrics(t *testing.T) {
	srv, st := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// создаём run напрямую (без HTTP — быстрее)
	if err := st.SaveRun(ctx, &models.Run{ID: "run-1", Status: models.StateRunning}); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/runs/run-1/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // адрес httptest-сервера, не SSRF
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Logf("close response body: %v", cerr)
		}
	})
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}

	// пишем метрику напрямую в store — SSE-хендлер обязан её доставить
	if err := st.AddMetrics(ctx, "run-1", &models.MetricBucket{RunID: "run-1", RPC: 42}); err != nil {
		t.Fatal(err)
	}

	line := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		if sc.Scan() {
			line <- sc.Text()
		}
	}()
	select {
	case got := <-line:
		if !strings.Contains(got, "data:") || !strings.Contains(got, `"rps":42`) {
			t.Fatalf("unexpected SSE line: %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for SSE event")
	}
}

func TestRegisterAgentEndpoint(t *testing.T) {
	srv, _ := setup(t)

	req := mustReq(t, http.MethodPost, srv.URL+"/api/agents", `{"version":"test","capacity":100}`)
	req.Header.Set("Content-Type", "application/json")
	//nolint:gosec // адрес httptest-сервера, не SSRF
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Logf("close response body: %v", cerr)
		}
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register agent: status=%d", resp.StatusCode)
	}
	var a models.Agent
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		t.Fatal(err)
	}
	if a.ID == "" || a.Version != "test" || a.Capacity != 100 || a.Status != models.UserOnline {
		t.Fatalf("unexpected agent: %+v", a)
	}
}

func TestGetRunConfigEndpoint(t *testing.T) {
	srv, st := setup(t)
	ctx := context.Background()

	scenarioYaml := "config: {vus: 2, duration: 5, rate: 10, name: smoke}\nsteps: [{method: GET, url: https://example.com}]"
	if err := st.SaveTest(ctx, &models.Test{
		ID:           "test-1",
		Name:         "smoke",
		ScenarioYaml: scenarioYaml,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveRun(ctx, &models.Run{
		ID:          "run-1",
		TestID:      "test-1",
		Status:      models.StateRunning,
		VUs:         2,
		DurationSec: 5,
		Rate:        10,
		Seed:        42,
	}); err != nil {
		t.Fatal(err)
	}

	//nolint:gosec // адрес httptest-сервера, не SSRF
	resp, err := http.DefaultClient.Do(mustReq(t, http.MethodGet, srv.URL+"/api/runs/run-1/config", ""))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Logf("close response body: %v", cerr)
		}
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get run config: status=%d", resp.StatusCode)
	}
	var cfg map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["run_id"] != "run-1" || cfg["scenario"] != scenarioYaml {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg["vus"] != float64(2) || cfg["duration"] != float64(5) {
		t.Fatalf("unexpected numeric params: %+v", cfg)
	}
	if cfg["rate"] != float64(10) || cfg["seed"] != float64(42) {
		t.Fatalf("unexpected numeric params: %+v", cfg)
	}
}

func TestGetRunConfigEndpointRunNotFound(t *testing.T) {
	srv, _ := setup(t)
	//nolint:gosec // адрес httptest-сервера, не SSRF
	resp, err := http.DefaultClient.Do(mustReq(t, http.MethodGet, srv.URL+"/api/runs/no-such/config", ""))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Logf("close response body: %v", cerr)
		}
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func mustReq(t *testing.T, method, url, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return req
}
