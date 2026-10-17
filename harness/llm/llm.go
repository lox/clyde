// Package llm is the only place that constructs model providers. Everything
// else takes an agent-harness Provider plus the model and effort to ask for.
package llm

import (
	"fmt"
	"os"

	harness "github.com/lox/agent-harness"
	"github.com/lox/agent-harness/provider/anthropic"
	"github.com/lox/agent-harness/provider/openai"
)

// Defaults for the executor slot.
const (
	DefaultProvider = "openai"
	DefaultModel    = "gpt-6.1-sol"
	DefaultEffort   = "low"
)

// Config picks a provider, model, and reasoning effort.
type Config struct {
	Provider string // openai (Responses API) or anthropic
	Model    string
	Effort   string // provider reasoning effort, e.g. low, medium, high
}

// ExecutorConfig reads CLYDE_EXECUTOR_PROVIDER, CLYDE_EXECUTOR_MODEL, and
// CLYDE_EXECUTOR_EFFORT, falling back to the defaults.
func ExecutorConfig() Config {
	return Config{
		Provider: env("CLYDE_EXECUTOR_PROVIDER", DefaultProvider),
		Model:    env("CLYDE_EXECUTOR_MODEL", DefaultModel),
		Effort:   env("CLYDE_EXECUTOR_EFFORT", DefaultEffort),
	}
}

// New constructs the provider. API keys come from OPENAI_API_KEY or
// ANTHROPIC_API_KEY; a missing key is an error rather than a fallback to
// another provider.
func New(cfg Config) (harness.Provider, error) {
	switch cfg.Provider {
	case "openai":
		key := os.Getenv("OPENAI_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("llm: OPENAI_API_KEY is not set")
		}
		return openai.New(openai.WithAPIKey(key), openai.WithDefaultModel(cfg.Model)), nil
	case "anthropic":
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("llm: ANTHROPIC_API_KEY is not set")
		}
		return anthropic.New(anthropic.WithAPIKey(key), anthropic.WithDefaultModel(cfg.Model)), nil
	}
	return nil, fmt.Errorf("llm: unknown provider %q (want openai or anthropic)", cfg.Provider)
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
