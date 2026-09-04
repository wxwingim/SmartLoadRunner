// Package store предоставляет хранилища данных для coordinator.
// InMemoryStore — потокобезопасное in-memory хранилище
// и pub/sub для метрик запусков.
package store

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/wxwingim/SmartLoadRunner/internal/models"
)

var (
	// ErrNotFound — сущность не найдена (тест, run, метрики к run).
	ErrNotFound = errors.New("store: not found")
	// ErrAlreadyExists — сущность с таким ID уже есть.
	ErrAlreadyExists = errors.New("store: already exists")
)

// Storage — контракт хранилища, которым пользуется HTTP API координатора.
type Storage interface {
	// SaveTest сохраняет тест; ErrAlreadyExists при дубликате ID.
	SaveTest(ctx context.Context, t *models.Test) error
	// GetTest возвращает тест или ErrNotFound.
	GetTest(ctx context.Context, id string) (*models.Test, error)
	// SaveRun сохраняет запуск; ErrAlreadyExists при дубликате ID.
	SaveRun(ctx context.Context, r *models.Run) error
	// GetRun возвращает запуск или ErrNotFound.
	GetRun(ctx context.Context, id string) (*models.Run, error)
	// AddMetrics сохраняет метрику и публикует её подписчикам runID.
	AddMetrics(ctx context.Context, runID string, m *models.MetricBucket) error
	// GetMetrics возвращает копию метрик runID или ErrNotFound.
	GetMetrics(ctx context.Context, runID string) ([]*models.MetricBucket, error)
	// Subscribe подписывается на метрики runID и возвращает канал и cancel-функцию.
	Subscribe(ctx context.Context, runID string) (<-chan *models.MetricBucket, func(), error)
	// RegisterAgent сохраняет агента; ErrAlreadyExists при дубликате ID.
	RegisterAgent(ctx context.Context, a *models.Agent) error
}

// InMemoryStore — потокобезопасное in-memory хранилище и pub/sub метрик.
type InMemoryStore struct {
	mu      sync.RWMutex
	tests   map[string]*models.Test
	runs    map[string]*models.Run
	agents  map[string]*models.Agent
	metrics map[string][]*models.MetricBucket
	subs    map[string]map[string]chan *models.MetricBucket // runID -> subID -> ch
	subMu   sync.Mutex
	subSeq  uint64 // генератор ID подписчиков
}

// compile-time: InMemoryStore реализует Storage.
var _ Storage = (*InMemoryStore)(nil)

// NewInMemoryStore создаёт новое хранилище с инициализированными картами.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		tests:   make(map[string]*models.Test),
		runs:    make(map[string]*models.Run),
		agents:  make(map[string]*models.Agent),
		metrics: make(map[string][]*models.MetricBucket),
		subs:    make(map[string]map[string]chan *models.MetricBucket),
	}
}

// SaveTest сохраняет тест в хранилище.
func (s *InMemoryStore) SaveTest(_ context.Context, t *models.Test) error {
	if t == nil {
		return fmt.Errorf("store: save test: nil test")
	}
	if t.ID == "" {
		return fmt.Errorf("store: save test: empty id")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tests[t.ID]; exists {
		return fmt.Errorf("%w: test %q", ErrAlreadyExists, t.ID)
	}
	s.tests[t.ID] = t
	return nil
}

// GetTest возвращает тест по ID.
func (s *InMemoryStore) GetTest(_ context.Context, id string) (*models.Test, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tests[id]
	if !ok {
		return nil, fmt.Errorf("%w: test %q", ErrNotFound, id)
	}
	return t, nil
}

// SaveRun сохраняет запуск.
func (s *InMemoryStore) SaveRun(_ context.Context, r *models.Run) error {
	if r == nil {
		return fmt.Errorf("store: save run: nil run")
	}
	if r.ID == "" {
		return fmt.Errorf("store: save run: empty id")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.runs[r.ID]; exists {
		return fmt.Errorf("%w: run %q", ErrAlreadyExists, r.ID)
	}
	s.runs[r.ID] = r
	return nil
}

// GetRun возвращает запуск по ID.
func (s *InMemoryStore) GetRun(_ context.Context, id string) (*models.Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.runs[id]
	if !ok {
		return nil, fmt.Errorf("%w: run %q", ErrNotFound, id)
	}
	return r, nil
}

// subscribersSnapshot возвращает копию каналов подписчиков runID под subMu.
// Копия нужна, чтобы потом рассылать ВНЕ s.mu (иначе — дедлок с GetMetrics).
func (s *InMemoryStore) subscribersSnapshot(runID string) []chan *models.MetricBucket {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	subs := s.subs[runID]
	if len(subs) == 0 {
		return nil
	}
	out := make([]chan *models.MetricBucket, 0, len(subs))
	for _, ch := range subs {
		out = append(out, ch)
	}
	return out
}

// AddMetrics добавляет метрики к запуску.
func (s *InMemoryStore) AddMetrics(_ context.Context, runID string, m *models.MetricBucket) error {
	// снимок подписчиков под отдельным мутексом (без вложенных блокировок)
	subs := s.subscribersSnapshot(runID)

	// атомарно с проверкой существования run делаем append под s.mu
	s.mu.Lock()
	if _, ok := s.runs[runID]; !ok {
		s.mu.Unlock()
		return fmt.Errorf("%w: run %q", ErrNotFound, runID)
	}
	s.metrics[runID] = append(s.metrics[runID], m)
	s.mu.Unlock()

	// неблокирующая публикация подписчикам вне блокировок
	for _, ch := range subs {
		select {
		case ch <- m:
		default: // медленный/отвалившийся SSE-клиент — дропаем, не блокируемся
		}
	}
	return nil
}

// GetMetrics возвращает метрики запуска (копию слайса).
func (s *InMemoryStore) GetMetrics(_ context.Context, runID string) ([]*models.MetricBucket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.runs[runID]; !ok {
		return nil, fmt.Errorf("%w: run %q", ErrNotFound, runID)
	}
	// копия: защищает от конкурентных append-ов в AddMetrics.
	out := make([]*models.MetricBucket, len(s.metrics[runID]))
	copy(out, s.metrics[runID])
	return out, nil
}

// Subscribe подписывается на метрики runID.
// Возвращает канал и cancel-функцию; повторный вызов cancel безопасен.
func (s *InMemoryStore) Subscribe(_ context.Context, runID string) (<-chan *models.MetricBucket, func(), error) {
	if runID == "" {
		return nil, nil, fmt.Errorf("store: subscribe: empty run id")
	}

	ch := make(chan *models.MetricBucket, 16) // буфер: меньше дропов при всплесках

	s.subMu.Lock()
	s.subSeq++
	subID := fmt.Sprintf("sub-%d", s.subSeq)
	if s.subs[runID] == nil {
		s.subs[runID] = make(map[string]chan *models.MetricBucket)
	}
	s.subs[runID][subID] = ch
	s.subMu.Unlock()

	// cancel удаляет подписку из map, НО НЕ закрывает канал:
	// канал никто не закрывает, иначе конкурентная публикация в AddMetrics
	// упадёт с panic: send on closed channel. Достаточно потерять ссылку — GC подберёт.
	cancel := func() {
		s.subMu.Lock()
		defer s.subMu.Unlock()
		delete(s.subs[runID], subID)
		if len(s.subs[runID]) == 0 {
			delete(s.subs, runID)
		}
	}

	return ch, cancel, nil
}

// RegisterAgent сохраняет агента; ErrAlreadyExists при дубликате ID.
func (s *InMemoryStore) RegisterAgent(_ context.Context, a *models.Agent) error {
	if a == nil {
		return fmt.Errorf("store: register agent: nil agent")
	}
	if a.ID == "" {
		return fmt.Errorf("store: register agent: empty id")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.agents[a.ID]; exists {
		return fmt.Errorf("%w: agent %q", ErrAlreadyExists, a.ID)
	}
	s.agents[a.ID] = a
	return nil
}

// GetAgent возвращает агента по ID или ErrNotFound.
func (s *InMemoryStore) GetAgent(_ context.Context, id string) (*models.Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.agents[id]
	if !ok {
		return nil, fmt.Errorf("%w: agent %q", ErrNotFound, id)
	}
	return a, nil
}

// SetAgentStatus обновляет статус агента или возвращает ErrNotFound.
func (s *InMemoryStore) SetAgentStatus(_ context.Context, id string, status models.OnlineStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.agents[id]
	if !ok {
		return fmt.Errorf("%w: agent %q", ErrNotFound, id)
	}
	a.Status = status
	return nil
}
