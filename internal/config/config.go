// Package config предоставляет единый механизм конфигурации для обоих бинарников
// (coordinator и runner). Конфигурация собирается из переменных окружения
// с префиксом SLR_ и может быть расширена флагами командной строки на уровне main.
//
// Структура разделена на секции:
//   - AppConfig   — базовые метаданные приложения
//   - LogConfig   — настройки логирования
//   - HTTPConfig  — HTTP API (использует coordinator)
//   - DBConfig    — PostgreSQL (использует coordinator, задел)
//   - RedisConfig — очереди/cache (задел)
//   - RunnerConfig — параметры воркера нагрузки
package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

// Config — корневая конфигурация приложения.
type Config struct {
	App    AppConfig
	Log    LogConfig
	HTTP   HTTPConfig
	DB     DBConfig
	Redis  RedisConfig
	Runner RunnerConfig
}

// AppConfig — базовые метаданные приложения.
type AppConfig struct {
	// Environment — среда выполнения: development, staging, production.
	Environment string `env:"SLR_ENVIRONMENT" envDefault:"development"`
	// Name — имя приложения.
	Name string `env:"SLR_APP_NAME" envDefault:"smart-load-runner"`
	// Version — версия сборки.
	Version string `env:"SLR_APP_VERSION" envDefault:"dev"`
}

// LogConfig — настройки логирования.
type LogConfig struct {
	// Level — уровень: debug, info, warn, error.
	Level string `env:"SLR_LOG_LEVEL" envDefault:"info"`
	// Format — формат вывода: console или json.
	Format string `env:"SLR_LOG_FORMAT" envDefault:"console"`
}

// HTTPConfig — настройки HTTP API координатора.
type HTTPConfig struct {
	Host         string `env:"SLR_HTTP_HOST"              envDefault:"0.0.0.0"`
	Port         int    `env:"SLR_HTTP_PORT"              envDefault:"8080"`
	ReadTimeout  int    `env:"SLR_HTTP_READ_TIMEOUT_SEC"  envDefault:"15"`
	WriteTimeout int    `env:"SLR_HTTP_WRITE_TIMEOUT_SEC" envDefault:"30"`
	IdleTimeout  int    `env:"SLR_HTTP_IDLE_TIMEOUT_SEC"  envDefault:"60"`
}

// DBConfig — PostgreSQL.
// Используется координатором для хранения тестов, снапшотов и результатов.
// На этапе MVP может быть заменен in-memory хранилищем (см. internal/store).
type DBConfig struct {
	Host     string `env:"SLR_DB_HOST"     envDefault:"localhost"`
	Port     int    `env:"SLR_DB_PORT"     envDefault:"5432"`
	User     string `env:"SLR_DB_USER"     envDefault:"slr"`
	Password string `env:"SLR_DB_PASSWORD" envDefault:"slr"` //nolint:gosec // значение по умолчанию для dev-окружения
	Name     string `env:"SLR_DB_NAME"     envDefault:"slr"`
	SSLMode  string `env:"SLR_DB_SSLMODE"  envDefault:"disable"`
	// MaxOpenConns — максимум одновременных соединений.
	MaxOpenConns int `env:"SLR_DB_MAX_OPEN_CONNS" envDefault:"10"`
	// MaxIdleConns — максимум простаивающих соединений.
	MaxIdleConns int `env:"SLR_DB_MAX_IDLE_CONNS" envDefault:"5"`
	// ConnMaxLifetime — время жизни соединения, минут.
	ConnMaxLifetime int `env:"SLR_DB_CONN_MAX_LIFETIME_MIN" envDefault:"30"`
}

// RedisConfig — Redis для очередей и кеша.
// Задел: координатор → runner коммуникация через очередь, кеш результатов.
type RedisConfig struct {
	Host     string `env:"SLR_REDIS_HOST"     envDefault:"localhost"`
	Port     int    `env:"SLR_REDIS_PORT"     envDefault:"6379"`
	Password string `env:"SLR_REDIS_PASSWORD" envDefault:""` //nolint:gosec // dev-окружение, пустой пароль
	DB       int    `env:"SLR_REDIS_DB"       envDefault:"0"`
}

// RunnerConfig — параметры воркера нагрузки.
// Runner читает только эту секцию.
type RunnerConfig struct {
	// ListenAddr — адрес gRPC/HTTP сервера агента (задел).
	ListenAddr string `env:"SLR_RUNNER_LISTEN_ADDR" envDefault:":9090"`
	// CoordinatorAddr — адрес координатора, к которому агент подключается.
	CoordinatorAddr string `env:"SLR_RUNNER_COORDINATOR_ADDR" envDefault:"localhost:8080"`
	// Capacity — максимальное число одновременных VU на агенте.
	Capacity int `env:"SLR_RUNNER_CAPACITY" envDefault:"1000"`
	// ReportIntervalSec — интервал отправки метрик координатору.
	ReportIntervalSec int `env:"SLR_RUNNER_REPORT_INTERVAL_SEC" envDefault:"1"`
	// DefaultScenarioFile — путь к YAML-сценарию по умолчанию (для локального запуска).
	DefaultScenarioFile string `env:"SLR_RUNNER_SCENARIO_FILE" envDefault:""`
}

// Dialect — тип для будущих адаптеров БД.
type Dialect string

const (
	// DialectPostgres — PostgreSQL.
	DialectPostgres Dialect = "postgres"
	// DialectSQLite — SQLite (для тестов и embedded-режима).
	DialectSQLite Dialect = "sqlite"
)

// Load парсит переменные окружения в Config.
// Используйте Load для обоих процессов: каждый бинарник читает только свои поля.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// Addr возвращает host:port для HTTP-сервера.
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.HTTP.Host, c.HTTP.Port)
}

// DSN возвращает строку подключения к PostgreSQL.
func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.DB.Host, c.DB.Port, c.DB.User, c.DB.Password, c.DB.Name, c.DB.SSLMode,
	)
}

// RedisAddr возвращает host:port для Redis.
func (c *Config) RedisAddr() string {
	return fmt.Sprintf("%s:%d", c.Redis.Host, c.Redis.Port)
}
