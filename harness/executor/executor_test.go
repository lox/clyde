package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	harness "github.com/lox/agent-harness"

	"github.com/lox/clyde/harness/body"
	"github.com/lox/clyde/harness/goal"
	"github.com/lox/clyde/harness/judge"
)

// fakeBody is an httptest body: actions finish instantly, collect adds four
// oak logs, and health is whatever the test sets.
type fakeBody struct {
	mu      sync.Mutex
	health  float64
	logs    int
	nextID  int
	last    map[string]any
	actions []string
	stops   int
}

func (b *fakeBody) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	reply := func(v any) { json.NewEncoder(w).Encode(v) }
	switch r.Method + " " + r.URL.Path {
	case "GET /state":
		if r.URL.Query().Get("format") == "text" {
			fmt.Fprintf(w, "bot Clyde | health %g/20 | inventory: oak_log x%d\n", b.health, b.logs)
			return
		}
		items := []any{}
		if b.logs > 0 {
			items = append(items, map[string]any{"name": "oak_log", "count": b.logs})
		}
		reply(map[string]any{
			"ts": time.Now().UnixMilli(),
			"bot": map[string]any{"name": "Clyde", "dimension": "overworld", "position": map[string]any{"x": 0, "y": 64, "z": 0},
				"yaw": 0, "pitch": 0, "health": b.health, "food": 20, "oxygen": 20, "xpLevel": 0, "gameMode": "survival",
				"onGround": true, "inWater": false, "inLava": false},
			"world":      map[string]any{"timeOfDay": 6000, "phase": "day", "raining": false, "thundering": false},
			"action":     nil,
			"lastAction": b.last,
			"inventory":  map[string]any{"used": len(items), "total": 36, "held": nil, "items": items},
			"entities":   map[string]any{"players": []any{}, "hostiles": []any{}, "passives": []any{}, "items": []any{}},
			"blocks":     []any{},
			"events":     []any{},
		})
	case "POST /action":
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		typ, _ := req["type"].(string)
		b.actions = append(b.actions, typ)
		if typ == "collect" {
			b.logs += 4
		}
		b.nextID++
		status := map[string]any{"id": b.nextID, "type": typ, "params": map[string]any{}, "status": "running", "startedAt": 1, "elapsedMs": 0}
		reply(status)
		b.last = map[string]any{"id": b.nextID, "type": typ, "params": map[string]any{}, "status": "succeeded", "startedAt": 1, "elapsedMs": 5}
	case "POST /stop":
		b.stops++
		reply(map[string]any{"stopped": nil})
	case "GET /events":
		reply(map[string]any{"events": []any{}, "latestSeq": 0, "missed": 0})
	default:
		w.WriteHeader(http.StatusNotFound)
		reply(map[string]any{"error": "no route " + r.URL.Path})
	}
}

func (b *fakeBody) snapshot() (actions []string, stops int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.actions...), b.stops
}

// scripted is a fake agent-harness provider: next decides each response
// from the call number and the conversation so far.
type scripted struct {
	delay time.Duration
	next  func(n int, p harness.ChatParams) *harness.ChatResult

	mu    sync.Mutex
	calls []harness.ChatParams
}

func (s *scripted) Chat(ctx context.Context, p harness.ChatParams) (*harness.ChatResult, error) {
	s.mu.Lock()
	n := len(s.calls)
	s.calls = append(s.calls, p)
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(s.delay):
	}
	return s.next(n, p), nil
}

func (s *scripted) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func toolCall(n int, name, args string) *harness.ChatResult {
	return &harness.ChatResult{
		Message: harness.Message{Role: harness.RoleAssistant, ToolCalls: []harness.ToolCall{
			{ID: fmt.Sprintf("call-%d", n), Name: name, Arguments: json.RawMessage(args)},
		}},
		FinishReason: harness.FinishReasonToolUse,
		Usage:        &harness.Usage{InputTokens: 1000, OutputTokens: 50},
	}
}

func say(text string) *harness.ChatResult {
	return &harness.ChatResult{
		Message:      harness.Message{Role: harness.RoleAssistant, Content: text},
		FinishReason: harness.FinishReasonEndTurn,
	}
}

var fourLogs = goal.Goal{
	Description: "Collect 4 oak logs.",
	Until:       goal.Predicate{Items: &goal.ItemsPredicate{Item: "minecraft:oak_log", Count: 4}},
}

func setup(t *testing.T, fb *fakeBody, p harness.Provider) Options {
	t.Helper()
	srv := httptest.NewServer(fb)
	t.Cleanup(srv.Close)
	c := &body.Client{URL: srv.URL}
	return Options{
		Provider: p, Model: "gpt-6.1-sol", Body: c, Observer: body.Observer{Client: c}, Player: "Clyde",
		SkillsDir: t.TempDir(), MaxSteps: 20, MaxDuration: 10 * time.Second, CheckInterval: 50 * time.Millisecond,
	}
}

func TestUntilHoldingMidRunSucceedsEarly(t *testing.T) {
	fb := &fakeBody{health: 20}
	p := &scripted{next: func(n int, _ harness.ChatParams) *harness.ChatResult {
		if n == 0 {
			return toolCall(n, "act", `{"type":"collect","block":"oak_log","count":4}`)
		}
		return toolCall(n, "observe", `{}`) // keeps going; the check must end the run
	}}
	o := setup(t, fb, p)

	out, err := Run(context.Background(), fourLogs, o)
	if err != nil {
		t.Fatal(err)
	}
	if out.Outcome != Succeeded || out.Until != "4/4 minecraft:oak_log" {
		t.Fatalf("outcome = %+v, want succeeded with 4/4", out)
	}
	// The check after the collect cancels the run before a second model call.
	if out.Steps != 1 || p.callCount() != 1 {
		t.Fatalf("steps %d, provider calls %d; want the run to stop after the first tool call", out.Steps, p.callCount())
	}
	if out.Tokens.Total != 1050 || out.CostUSD <= 0 {
		t.Fatalf("usage not recorded: %+v cost %v", out.Tokens, out.CostUSD)
	}
	if actions, stops := fb.snapshot(); len(actions) != 1 || actions[0] != "collect" || stops != 1 {
		t.Fatalf("body saw actions %v and %d stops, want [collect] and 1 stop", actions, stops)
	}
}

func TestJudgeInterruptCancelsAndStops(t *testing.T) {
	fb := &fakeBody{health: 3} // the low_health pre-check fires on the first tick
	p := &scripted{delay: 20 * time.Millisecond, next: func(n int, _ harness.ChatParams) *harness.ChatResult {
		return toolCall(n, "observe", `{}`)
	}}
	o := setup(t, fb, p)
	o.MaxSteps = 1000
	cfg := judge.DefaultConfig()
	cfg.Rate = 20
	o.Judge = judge.New(judge.BodySource{URL: o.Body.URL}, unusedBackend{}, nil, cfg)

	start := time.Now()
	out, err := Run(context.Background(), fourLogs, o)
	if err != nil {
		t.Fatal(err)
	}
	if out.Outcome != Interrupted || out.Interrupt == nil || out.Interrupt.Reason != "danger" || out.Interrupt.Source != judge.SourcePrecheck {
		t.Fatalf("outcome = %+v, want interrupted by the danger pre-check", out)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("interrupt took %s", time.Since(start))
	}
	if _, stops := fb.snapshot(); stops != 1 {
		t.Fatalf("%d stops, want 1", stops)
	}
}

func TestRecoveryJudgeCanContinueOrInterruptFallingHealth(t *testing.T) {
	for _, verdict := range []string{"none", "danger"} {
		t.Run(verdict, func(t *testing.T) {
			fb := &fakeBody{health: 3}
			seen := make(chan struct{}, 1)
			p := &scripted{delay: 20 * time.Millisecond, next: func(n int, _ harness.ChatParams) *harness.ChatResult {
				select {
				case <-seen:
				case <-time.After(time.Second):
					t.Error("recovery evidence never reached judge")
				}
				return toolCall(n, "act", `{"type":"collect","block":"oak_log","count":4}`)
			}}
			o := setup(t, fb, p)
			o.Context = ExecutionContext{Mode: "recovery", Danger: judge.Danger{{Name: "low_health", Fired: true, Value: 5, Threshold: 6}}}
			o.Judge = judge.New(judge.BodySource{URL: o.Body.URL}, recoveryBackend{reason: verdict, seen: seen}, nil, judge.DefaultConfig())
			out, err := Run(context.Background(), fourLogs, o)
			if err != nil {
				t.Fatal(err)
			}
			if verdict == "none" && out.Outcome != Succeeded {
				t.Fatalf("falling health vetoed useful recovery: %+v", out)
			}
			if verdict == "danger" && (out.Outcome != Interrupted || out.Interrupt == nil || out.Interrupt.Source != judge.SourceModel || out.Interrupt.Reason != "danger") {
				t.Fatalf("active recovery judge did not interrupt: %+v", out)
			}
			if _, stops := fb.snapshot(); stops != 1 {
				t.Fatalf("cleanup stops=%d want 1", stops)
			}
		})
	}
}

type recoveryBackend struct {
	reason string
	seen   chan struct{}
}

func (recoveryBackend) Name() string { return "scripted" }
func (b recoveryBackend) Ask(context.Context, json.RawMessage, map[string]judge.Question) (judge.Result, error) {
	select {
	case b.seen <- struct{}{}:
	default:
	}
	confidence := 1.0
	return judge.Result{Answers: map[string]judge.Answer{"interrupt": {Type: "choice", Choice: b.reason, Confidence: &confidence}}}, nil
}

// unusedBackend fails if asked; pre-checks decide the tests that use it.
type unusedBackend struct{}

func (unusedBackend) Name() string { return "unused" }

func (unusedBackend) Ask(context.Context, json.RawMessage, map[string]judge.Question) (judge.Result, error) {
	return judge.Result{}, fmt.Errorf("backend should not be asked")
}

func TestMaxStepsIsBudgetExhausted(t *testing.T) {
	fb := &fakeBody{health: 20}
	p := &scripted{next: func(n int, _ harness.ChatParams) *harness.ChatResult {
		return toolCall(n, "observe", `{}`)
	}}
	o := setup(t, fb, p)
	o.MaxSteps = 3

	out, err := Run(context.Background(), fourLogs, o)
	if err != nil {
		t.Fatal(err)
	}
	if out.Outcome != BudgetExhausted || out.Steps != 3 || out.Detail != "max steps 3" {
		t.Fatalf("outcome = %+v, want budget_exhausted after 3 steps", out)
	}
	if _, stops := fb.snapshot(); stops != 1 {
		t.Fatalf("%d stops, want 1", stops)
	}
}

func TestBadToolArgumentsAreToolErrors(t *testing.T) {
	fb := &fakeBody{health: 20}
	bad := []struct{ tool, args, want string }{
		{"act", `{"type":"goto","x":"far","y":64,"z":0}`, "invalid goto action"},
		{"act", `{"type":"goto","x":1,"y":64}`, "field z in GotoAction: required"},
		{"act", `{"type":"fly"}`, "type must be one of"},
		{"act", `{"type":"collect","block":"oak_log","count":99}`, "count"},
		{"act", `not json`, "bad arguments"},
		{"eval_js", `{}`, "code is required"},
		{"run_skill", `{"name":"missing","args":{}}`, "no such file"},
		{"teleport", `{}`, "Unknown tool"},
	}
	p := &scripted{next: func(n int, _ harness.ChatParams) *harness.ChatResult {
		if n < len(bad) {
			return toolCall(n, bad[n].tool, bad[n].args)
		}
		return say("I cannot do this.")
	}}
	o := setup(t, fb, p)

	out, err := Run(context.Background(), fourLogs, o)
	if err != nil {
		t.Fatal(err)
	}
	if out.Outcome != Failed || out.Final != "I cannot do this." {
		t.Fatalf("outcome = %+v, want failed after the model gives up", out)
	}
	// The last provider call sees every earlier tool result.
	msgs := p.calls[len(p.calls)-1].Messages
	var results []*harness.ToolResult
	for _, m := range msgs {
		if m.Role == harness.RoleTool {
			results = append(results, m.ToolResult)
		}
	}
	if len(results) != len(bad) {
		t.Fatalf("%d tool results, want %d", len(results), len(bad))
	}
	for i, r := range results {
		if !r.IsError || !strings.Contains(r.Content, bad[i].want) {
			t.Errorf("%s %s: result %q (error %v), want an error containing %q", bad[i].tool, bad[i].args, r.Content, r.IsError, bad[i].want)
		}
	}
	if actions, _ := fb.snapshot(); len(actions) != 0 {
		t.Fatalf("invalid actions reached the body: %v", actions)
	}
	if out.SkillCalls["missing"] != 1 {
		t.Fatalf("attempted skill usage not reported: %+v", out.SkillCalls)
	}
}
