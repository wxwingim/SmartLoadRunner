package store

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wxwingim/SmartLoadRunner/internal/models"
	"github.com/wxwingim/SmartLoadRunner/internal/scenario"
)

// Persistence — долговременное хранение метадаты на диске.
type Persistence struct {
	testsDir string
	runsDir  string
	mu       sync.Mutex // сериализация JSONL-записей
}

// NewPersistence создаёт Persistence и директории data/tests и data/runs.
func NewPersistence(testsDir, runsDir string) (*Persistence, error) {
	for _, dir := range []string{testsDir, runsDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	return &Persistence{testsDir: testsDir, runsDir: runsDir}, nil
}

// SaveTest пишет сценарий в data/tests/<id>.yaml (атомарно).
func (p *Persistence) SaveTest(t *models.Test) error {
	path := filepath.Join(p.testsDir, t.ID+".yaml")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(t.ScenarioYaml), 0o600); err != nil {
		return fmt.Errorf("write test %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename test %s: %w", path, err) // атомарная запись
	}
	return nil
}

// SaveRun пишет метадату запуска в data/runs/<id>.json.
func (p *Persistence) SaveRun(r *models.Run) error {
	data, err := json.MarshalIndent(r, "", "  ") // pretty — человекочитаемый JSON
	if err != nil {
		return fmt.Errorf("marshal run: %w", err)
	}
	path := filepath.Join(p.runsDir, r.ID+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write run %s: %w", path, err)
	}
	return nil
}

// AppendMetric — добавляет одну JSON-строку в <runID>.jsonl
func (p *Persistence) AppendMetric(runID string, m *models.MetricBucket) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	line, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal metric: %w", err)
	}
	path := filepath.Join(p.runsDir, runID+".jsonl")
	//nolint:gosec // runID приходит от координатора, не от пользователя
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open jsonl %s: %w", runID, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			slog.Warn("persistence: close jsonl", "run_id", runID, "error", cerr)
		}
	}()
	if _, err = f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append metric %s: %w", runID, err)
	}
	return nil
}

// LoadAll — на старте координатора поднимает метадату с диска в store.
func (p *Persistence) LoadAll(ctx context.Context, st *InMemoryStore) error {
	entries, err := os.ReadDir(p.testsDir)
	if err != nil {
		return fmt.Errorf("read tests dir %s: %w", p.testsDir, err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(p.testsDir, e.Name()))
		if err != nil {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".yaml")
		sc, err := scenario.Parse(data)
		if err != nil {
			continue // битый файл — не роняем старт, только пропускаем
		}
		if err := st.SaveTest(ctx, &models.Test{
			ID:           id,
			Name:         sc.Config.Name,
			ScenarioYaml: string(data),
			OwnerID:      "local",
			CreatedAt:    time.Now().UTC(),
		}); err != nil {
			continue
		}
	}
	return nil
}
