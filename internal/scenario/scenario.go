// Package scenario содержит модель и валидацию YAML-сценария нагрузочного теста.
package scenario

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Scenario — корневой элемент YAML-сценария.
type Scenario struct {
	Config Config `yaml:"config"`
	Steps  []Step `yaml:"steps"`
}

// Config — параметры нагрузки по умолчанию (переопределяются при старте run).
type Config struct {
	Name        string `yaml:"name"`
	VUs         int    `yaml:"vus"`
	DurationSec int    `yaml:"duration"`
	Rate        int    `yaml:"rate"`
	Seed        int64  `yaml:"seed"`
}

// Step — один шаг сценария (что делает VU).
type Step struct {
	Name      string            `yaml:"name"`
	Method    string            `yaml:"method"`
	URL       string            `yaml:"url"`
	Headers   map[string]string `yaml:"headers"`
	Body      string            `yaml:"body"`
	ThinkTime time.Duration     `yaml:"think_time"` // пауза между шагами
}

// Parse парсит YAML в Scenario.
func Parse(data []byte) (*Scenario, error) {
	var s Scenario
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse scenario yaml: %w", err)
	}
	return &s, nil
}

// Validate — лёгкая бизнес-валидация: до сохранения теста, до старта run.
func (s *Scenario) Validate() error {
	if s.Config.VUs <= 0 {
		return fmt.Errorf("config.vus must be > 0, got %d", s.Config.VUs)
	}
	if s.Config.DurationSec <= 0 {
		return fmt.Errorf("config.duration must be > 0")
	}
	for i, st := range s.Steps {
		if st.Method == "" || st.URL == "" {
			return fmt.Errorf("steps[%d]: method and url are required", i)
		}
	}
	return nil
}
