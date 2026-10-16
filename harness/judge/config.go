package judge

import (
	"errors"
	"fmt"
	"os"
	"time"

	"go.yaml.in/yaml/v3"
)

// Config is everything tunable about the judge except the questions, which
// live in prompts/judge.yaml.
type Config struct {
	// Rate is ticks per second.
	Rate float64 `yaml:"rate"`
	// Deadline bounds one tick: reading the card plus the backend call.
	Deadline  time.Duration  `yaml:"deadline"`
	Prechecks PrecheckConfig `yaml:"prechecks"`
	Decision  DecisionConfig `yaml:"decision"`
	// CostPerMTok is the backend's price per million input tokens, in USD,
	// used for the cost estimate in each record.
	CostPerMTok float64 `yaml:"cost_per_mtok"`
}

// DecisionConfig controls how model answers become interrupts.
type DecisionConfig struct {
	// MinConfidence: an interrupt choice below this confidence does not
	// interrupt on its own.
	MinConfidence float64 `yaml:"min_confidence"`
	// Corroborate: a low-confidence choice still interrupts when the noul
	// question of the same name (danger, stuck) is at or above this.
	Corroborate float64 `yaml:"corroborate"`
}

// DefaultConfig returns the defaults: 2 Hz, 800 ms deadline, Jev pricing.
func DefaultConfig() Config {
	return Config{
		Rate:     2,
		Deadline: 800 * time.Millisecond,
		Prechecks: PrecheckConfig{
			LowHealth:       6,
			LowOxygen:       6,
			HostileRange:    4,
			StuckAfter:      10 * time.Second,
			MoveEpsilon:     0.5,
			HealthyHealth:   14,
			AttentionEvents: []string{"chat", "whisper", "damage"},
		},
		Decision: DecisionConfig{
			MinConfidence: 0.5,
			Corroborate:   0.7,
		},
		CostPerMTok: 0.042,
	}
}

// LoadConfig overlays the YAML file at path, if any, on DefaultConfig.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	if cfg.Rate <= 0 || cfg.Deadline <= 0 {
		return cfg, errors.New("rate and deadline must be positive")
	}
	return cfg, nil
}
