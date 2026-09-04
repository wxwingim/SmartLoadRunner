package scenario

import (
	"strings"
	"testing"
)

// validYAML — эталонный сценарий: блок config + один шаг.
// Отступы — 2 пробела, не табы: иначе YAML-парсер прочитает блок неверно.
func validYAML() string {
	return strings.Join([]string{
		"config:",
		"  vus: 5",
		"  duration: 10",
		"  rate: 20",
		"steps:",
		"  - name: ping",
		"    method: GET",
		"    url: https://example.com/status",
	}, "\n")
}

func TestParse(t *testing.T) {
	s, err := Parse([]byte(validYAML()))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if s.Config.VUs != 5 || s.Config.DurationSec != 10 || s.Config.Rate != 20 {
		t.Fatalf("config mismatch: %+v", s.Config)
	}
	if len(s.Steps) != 1 || s.Steps[0].URL == "" {
		t.Fatalf("steps mismatch: %+v", s.Steps)
	}
}

func TestParseInvalidYAML(t *testing.T) {
	if _, err := Parse([]byte("config: [broken")); err == nil {
		t.Fatal("want error on broken yaml")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{
			"zero vus",
			"config: {vus: 0, duration: 5, rate: 1}\nsteps: [{method: GET, url: https://x}]",
		},
		{
			"zero duration",
			"config: {vus: 1, duration: 0, rate: 1}\nsteps: [{method: GET, url: https://x}]",
		},
		{
			"missing url",
			"config: {vus: 1, duration: 5, rate: 1}\nsteps: [{method: GET}]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Parse([]byte(tc.raw))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if err := s.Validate(); err == nil {
				t.Fatal("want validation error, got nil")
			}
		})
	}
}

func TestValidateOK(t *testing.T) {
	s, err := Parse([]byte(validYAML()))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("valid scenario must pass: %v", err)
	}
}

// TestScenarioContract — guard: не ломаем шаблон, на которой опираются хендлеры.
func TestScenarioContract(t *testing.T) {
	raw := "config: {vus: 2, duration: 5, rate: 10, seed: 7}\n" +
		"steps: [{name: ping, method: GET, url: https://x, think_time: 1s}]"
	s, err := Parse([]byte(raw))
	if err != nil || s.Validate() != nil {
		t.Fatalf("contract scenario must pass: %v", err)
	}
	if s.Config.Seed != 7 {
		t.Fatalf("seed mismatch: %d", s.Config.Seed)
	}
}
