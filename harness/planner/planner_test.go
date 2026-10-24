package planner_test

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	harness "github.com/lox/agent-harness"
	"github.com/lox/clyde/harness/llm"
	"github.com/lox/clyde/harness/planner"
)

type providerFunc func(context.Context, harness.ChatParams) (*harness.ChatResult, error)

func (f providerFunc) Chat(ctx context.Context, p harness.ChatParams) (*harness.ChatResult, error) {
	return f(ctx, p)
}
func answer(s string) (*harness.ChatResult, error) {
	return &harness.ChatResult{Message: harness.Message{Role: harness.RoleAssistant, Content: s}, FinishReason: harness.FinishReasonEndTurn, Usage: &harness.Usage{InputTokens: 100, OutputTokens: 20}}, nil
}
func model(t *testing.T, p harness.Provider) *planner.Planner {
	t.Helper()
	schema, err := os.ReadFile("../../api/goal.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := planner.New(planner.Options{Provider: p, Config: llm.Config{Model: "gpt-6.1-sol", Effort: "low"}, System: "Return a goal or idle.", GoalSchema: schema})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestPlanningValidationBoundedRetry(t *testing.T) {
	for _, bad := range []string{
		`{"goal":{"description":"bad","until":{"items":null}},"idle":false}`,
		`{"goal":{"description":"bad","until":{"items":{"item":"oak_log","count":0}}},"idle":false}`,
		`{"goal":{"description":"bad","until":{"items":{"item":"oak_log","count":1,"injected":true}}},"idle":false}`,
		`{"goal":{"description":"bad","until":{"items":{"item":"oak_log","count":1},"arrived":{"at":{"x":1,"y":2,"z":3},"radius":1}}},"idle":false}`,
		`{"goal":{"description":"bad","until":{"judged":{"question":"done?"}}},"idle":false}`,
		`{"idle":true,"goal":{"description":"bad","until":{}}}`,
		`{"idle":true,"work":"cancel"}`,
		`{"idle":false,"work":"preserve","reply":"no measurable goal"}`,
		`{"idle":true,"project":"` + strings.Repeat("x", 4097) + `"}`,
	} {
		t.Run(bad, func(t *testing.T) {
			var calls atomic.Int32
			m := model(t, providerFunc(func(context.Context, harness.ChatParams) (*harness.ChatResult, error) {
				calls.Add(1)
				return answer(bad)
			}))
			d, err := m.Plan(context.Background(), planner.Input{Event: "idle"})
			if err == nil || d.Goal != nil || calls.Load() != 2 || d.Tokens != 240 {
				t.Fatalf("decision=%+v error=%v calls=%d", d, err, calls.Load())
			}
		})
	}
	var calls atomic.Int32
	m := model(t, providerFunc(func(_ context.Context, p harness.ChatParams) (*harness.ChatResult, error) {
		if p.Model != "gpt-6.1-sol" {
			t.Errorf("model=%s", p.Model)
		}
		if calls.Add(1) == 1 {
			return answer(`not JSON`)
		}
		return answer(`{"idle":false,"goal":{"description":"Collect three birch logs","until":{"items":{"item":"minecraft:birch_log","count":3}}}}`)
	}))
	d, err := m.Plan(context.Background(), planner.Input{Event: "player_request", Request: "Alex: collect birch"})
	if err != nil || d.Goal == nil || d.Goal.Until.Items.Count != 3 || d.Tokens != 240 || d.CostUSD <= 0 {
		t.Fatalf("decision=%+v error=%v", d, err)
	}
}

func TestIndependentSlotConfiguration(t *testing.T) {
	t.Setenv("CLYDE_EXECUTOR_MODEL", "other-executor")
	t.Setenv("CLYDE_PLANNER_MODEL", "gpt-6.1-sol")
	t.Setenv("CLYDE_PLANNER_EFFORT", "medium")
	cfg := planner.Config()
	if cfg.Model != "gpt-6.1-sol" || cfg.Effort != "medium" || !strings.Contains(llm.ExecutorConfig().Model, "other-executor") {
		t.Fatalf("planner=%+v executor=%+v", cfg, llm.ExecutorConfig())
	}
}
