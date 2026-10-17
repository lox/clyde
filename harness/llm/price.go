package llm

import harness "github.com/lox/agent-harness"

// Price is USD per million tokens, standard short-context rates.
type Price struct {
	Input, CachedInput, CacheWrite, Output float64
}

// Prices for the models we have run, from the providers' pricing pages
// (OpenAI, 2026-10-17). Add a model here before relying on its cost numbers.
var Prices = map[string]Price{
	"gpt-6-astra": {Input: 10, CachedInput: 1, CacheWrite: 12.5, Output: 50},
	"gpt-6.1-sol": {Input: 2, CachedInput: 0.10, CacheWrite: 2.5, Output: 10},
	"gpt-6-sol":   {Input: 2, CachedInput: 0.20, CacheWrite: 2.5, Output: 10},
	"gpt-6-luna":  {Input: 0.10, CachedInput: 0.01, CacheWrite: 0.125, Output: 0.50},
}

// Cost prices each provider call separately and sums them. ok is false when
// the model has no price.
func Cost(model string, calls []harness.Usage) (usd float64, ok bool) {
	p, ok := Prices[model]
	if !ok {
		return 0, false
	}
	for _, u := range calls {
		usd += (float64(u.InputTokens)*p.Input +
			float64(u.CachedInputTokens)*p.CachedInput +
			float64(u.CacheCreationInputTokens)*p.CacheWrite +
			float64(u.OutputTokens)*p.Output) / 1e6
	}
	return usd, true
}
