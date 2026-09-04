// Package models содержит доменные модели системы нагрузочного тестирования:
// тесты, снапшоты сценариев, запуски, агенты и метрики.
package models

import "time"

// RunState — состояние запуска (Run).
type RunState string

// OnlineStatus — статус присутствия агента.
type OnlineStatus string

// Состояния запусков.
const (
	StateDraft     RunState = "DRAFT"
	StateScheduled RunState = "SCHEDULED"
	StateStarting  RunState = "STARTING"
	StateRunning   RunState = "RUNNING"
	StateStopping  RunState = "STOPPING"
	StateCompleted RunState = "COMPLETED"
	StateFailed    RunState = "FAILED"
	StateAborted   RunState = "ABORTED"
)

// Статусы агентов.
const (
	UserRegistered OnlineStatus = "REGISTERED"
	UserOnline     OnlineStatus = "ONLINE"
	UserOffline    OnlineStatus = "OFFLINE"
)

// Test хранит исходный сценарий, метаданные и ссылки на версии.
type Test struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ScenarioYaml string    `json:"scenario_yaml"`
	OwnerID      string    `json:"owner_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// Snapshot — неизменяемый снапшот сценария для воспроизводимости результатов.
type Snapshot struct {
	ID string `json:"id"`
}

// Run — единица исполнения теста;
// содержит конфигурацию запуска, статус, ссылки на метрики и артефакты.
type Run struct {
	ID             string         `json:"id"`
	TestID         string         `json:"test_id"`
	Status         RunState       `json:"status"`
	VUs            int            `json:"vus"`
	DurationSec    int            `json:"duration"`
	Rate           int            `json:"rate"`
	Seed           int64          `json:"seed"`
	StartAt        time.Time      `json:"start_at,omitempty"`
	StopAt         time.Time      `json:"stop_at,omitempty"`
	MetricsSummary map[string]any `json:"metrics_summary,omitempty"`
}

// Agent — зарегистрированный воркер (runner), который выполняет VU.
type Agent struct {
	ID           string       `json:"id"`
	Version      string       `json:"version"`
	Capacity     int          `json:"capacity"`
	LastSeen     time.Time    `json:"last_seen"`
	Status       OnlineStatus `json:"status"`
	PublicKey    string       `json:"public_key"`
	RegisteredBy time.Time    `json:"registered_by"`
	Metadata     string       `json:"metadata"`
}

// RunAllocation — распределение нагрузки между агентами.
type RunAllocation struct{}

// MetricBucket — агрегированные метрики по запуску за интервал времени.
type MetricBucket struct {
	RunID     string    `json:"run_id"`
	Timestamp time.Time `json:"ts"`
	RPC       float64   `json:"rps"`
	P50       float64   `json:"p50"`
	P90       float64   `json:"p90"`
	P99       float64   `json:"p99"`
	Errors    int       `json:"errors"`
	ActiveVUs int       `json:"active_vus"`
}

// Artifact хранит логи, отчёты, CSV/JSON-экспорты и replay-пакеты.
type Artifact struct{}
