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
	mu           sync.Mutex
	health       float64
	logs         int
	nextID       int
	last         map[string]any
	current      map[string]any
	firstFailure string
	holdFirst    bool
	stopped      chan struct{}
	actions      []string
	stops        int
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
			"action":     b.current,
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
		if typ == "stop" {
			var stopped map[string]any
			if b.current != nil && b.current["token"] == req["token"] {
				stopped = b.current
				stopped["status"], stopped["error"] = "aborted", "stop action"
				b.last, b.current = stopped, nil
				if b.stopped != nil {
					close(b.stopped)
					b.stopped = nil
				}
			}
			reply(map[string]any{"stopped": stopped})
			return
		}
		b.actions = append(b.actions, typ)
		if typ == "collect" && !(b.nextID == 0 && (b.firstFailure != "" || b.holdFirst)) {
			b.logs += 4
		}
		b.nextID++
		status := map[string]any{"id": b.nextID, "token": fmt.Sprintf("owner-%d", b.nextID), "type": typ, "params": map[string]any{}, "status": "running", "startedAt": 1, "elapsedMs": 0}
		reply(status)
		b.last = map[string]any{"id": b.nextID, "token": fmt.Sprintf("owner-%d", b.nextID), "type": typ, "params": map[string]any{}, "status": "succeeded", "startedAt": 1, "elapsedMs": 5}
		if b.nextID == 1 && b.firstFailure != "" {
			b.last["status"], b.last["error"] = "failed", b.firstFailure
		}
		if b.nextID == 1 && b.holdFirst {
			b.current, b.last = status, nil
		}
	case "POST /stop":
		b.stops++
		if b.current != nil {
			b.current["status"] = "aborted"
			b.last, b.current = b.current, nil
		}
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

	mu        sync.Mutex
	calls     []harness.ChatParams
	deadlines []time.Time
}

func (s *scripted) Chat(ctx context.Context, p harness.ChatParams) (*harness.ChatResult, error) {
	s.mu.Lock()
	n := len(s.calls)
	s.calls = append(s.calls, p)
	deadline, _ := ctx.Deadline()
	s.deadlines = append(s.deadlines, deadline)
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

// A real judge and tool loop over HTTP: a terminal failure must reach the next
// model turn, while a live stall stops only its action, even after wait=0.
func TestActionFailuresContinueInOriginalConversation(t *testing.T) {
	for _, mode := range []string{"terminal", "live-waiting", "live-returned"} {
		t.Run(mode, func(t *testing.T) {
			stopped := make(chan struct{})
			fb := &fakeBody{health: 13, holdFirst: mode != "terminal", stopped: stopped}
			if mode == "terminal" {
				fb.firstFailure = "Took to long to decide path to goal!"
			}
			seen := make(chan struct{}, 1)
			var inference chan struct{}
			if mode == "live-returned" {
				inference = make(chan struct{})
			}
			p := &scripted{next: func(n int, params harness.ChatParams) *harness.ChatResult {
				if n == 0 {
					wait := 60
					if mode == "live-returned" {
						wait = 0
					}
					return toolCall(n, "act", fmt.Sprintf(`{"type":"collect","block":"oak_log","wait":%d}`, wait))
				}
				if mode == "terminal" && n == 1 {
					select {
					case <-seen:
					case <-time.After(time.Second):
						t.Error("terminal failure not judged")
					}
					time.Sleep(20 * time.Millisecond) // Let the verdict reach Run before the next action.
				}
				if mode == "live-returned" && n == 1 {
					close(inference) // The verdict must arrive during this inference, not before it.
					select {
					case <-stopped:
					case <-time.After(time.Second):
						t.Error("live action not stopped")
					}
					res := say("stale end-turn must not end this goal")
					res.Usage = &harness.Usage{InputTokens: 17, OutputTokens: 11}
					return res
				}
				text, _ := json.Marshal(params.Messages)
				if mode == "terminal" && !strings.Contains(string(text), fb.firstFailure) {
					t.Error("actual error missing from conversation")
				}
				if mode != "terminal" && !strings.Contains(string(text), "Same goal and run budget remain") {
					t.Error("stall reason missing from continuing conversation")
				}
				if !strings.Contains(string(text), "Current state") && !strings.Contains(string(text), "Fresh state") {
					t.Error("fresh observation missing")
				}
				return toolCall(n, "act", `{"type":"collect","block":"oak_log"}`)
			}}
			o := setup(t, fb, p)
			o.MaxSteps, o.MaxDuration = 40, 300*time.Second
			cfg := judge.DefaultConfig()
			cfg.Rate = 100
			o.Judge = judge.New(judge.BodySource{URL: o.Body.URL}, actionStuckBackend{seen: seen, inference: inference}, nil, cfg)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			out, err := Run(ctx, fourLogs, o)
			if err != nil || out.Outcome != Succeeded || out.Until != "4/4 minecraft:oak_log" {
				t.Fatalf("out=%+v err=%v", out, err)
			}
			wantSteps, wantTokens := 2, 2100
			if mode == "live-returned" {
				wantSteps, wantTokens = 3, 2128
			}
			if out.Steps != wantSteps || p.callCount() != wantSteps {
				t.Fatalf("conversation restarted or stale end-turn accepted: %+v calls=%d", out, p.callCount())
			}
			if out.Tokens.Total != wantTokens {
				t.Fatalf("returned usage lost across continuation: %+v want %d", out.Tokens, wantTokens)
			}
		})
	}
}

// Block transcript emission after exploringProvider returned, not inference.
// A later judge tick confirms its serial stop callback has fully completed.
func TestLateStuckBeforeTerminalAcceptance(t *testing.T) {
	for _, finish := range []harness.FinishReason{harness.FinishReasonEndTurn, harness.FinishReasonRefusal, harness.FinishReasonIncomplete, harness.FinishReasonMaxTokens} {
		for _, limit := range []int{4, 3, 2} {
			t.Run(fmt.Sprintf("%s/max_steps=%d", finish, limit), func(t *testing.T) {
				inference := make(chan struct{})
				settled := make(chan struct{}, 1)
				fb := &fakeBody{health: 13, holdFirst: true}
				p := &scripted{next: func(n int, params harness.ChatParams) *harness.ChatResult {
					switch n {
					case 0:
						return toolCall(n, "act", `{"type":"collect","block":"oak_log","wait":0}`)
					case 1:
						res := say("late terminal response")
						res.FinishReason = finish
						res.Usage = &harness.Usage{InputTokens: 17, OutputTokens: 11}
						return res
					default:
						text, _ := json.Marshal(params.Messages)
						if !strings.Contains(string(text), fourLogs.Description) || !strings.Contains(string(text), "owner-1") || !strings.Contains(string(text), "late terminal response") || !strings.Contains(string(text), "Same goal and run budget remain") {
							t.Error("continuation lost original conversation or stop evidence")
						}
						return toolCall(n, "act", `{"type":"collect","block":"oak_log"}`)
					}
				}}
				o := setup(t, fb, p)
				o.MaxSteps = limit
				o.Log = writeFunc(func(b []byte) (int, error) {
					if strings.Contains(string(b), "model: late terminal response") {
						close(inference)
						select {
						case <-settled:
						case <-time.After(time.Second):
							t.Error("judge did not finish stop after provider return")
						}
					}
					return len(b), nil
				})
				cfg := judge.DefaultConfig()
				cfg.Rate = 100
				o.Judge = judge.New(judge.BodySource{URL: o.Body.URL}, actionStuckBackend{seen: make(chan struct{}, 1), inference: inference, settled: settled}, nil, cfg)
				out, err := Run(context.Background(), fourLogs, o)
				wantOutcome, wantSteps, wantTokens, wantCost := Succeeded, 3, 2128, 0.005144
				if limit < 4 {
					wantOutcome = BudgetExhausted
				}
				if limit == 2 {
					wantSteps, wantTokens, wantCost = 2, 1078, 0.002644
				}
				if err != nil || out.Outcome != wantOutcome || out.Steps != wantSteps || p.callCount() != wantSteps || out.Tokens.Total != wantTokens || out.CostUSD != wantCost {
					t.Fatalf("out=%+v calls=%d err=%v; want %s, %d original turns, %d returned tokens, cost %g", out, p.callCount(), err, wantOutcome, wantSteps, wantTokens, wantCost)
				}
				if wantOutcome == Succeeded && out.Until != "4/4 minecraft:oak_log" {
					t.Fatalf("original goal not independently verified: %+v", out)
				}
				for _, deadline := range p.deadlines {
					if deadline.IsZero() || !deadline.Equal(p.deadlines[0]) {
						t.Fatalf("continuation changed original deadline: %v", p.deadlines)
					}
				}
			})
		}
	}
}

type writeFunc func([]byte) (int, error)

func (f writeFunc) Write(b []byte) (int, error) { return f(b) }

type actionStuckBackend struct {
	seen      chan struct{}
	inference <-chan struct{}
	settled   chan<- struct{}
}

func (actionStuckBackend) Name() string { return "action-stuck" }
func (b actionStuckBackend) Ask(ctx context.Context, raw json.RawMessage, _ map[string]judge.Question) (judge.Result, error) {
	var c body.Card
	if err := json.Unmarshal(raw, &c); err != nil {
		return judge.Result{}, err
	}
	reason := "none"
	if c.Action != nil || (c.LastAction != nil && c.LastAction.Status == "failed") {
		if b.inference != nil {
			select {
			case <-b.inference:
			case <-ctx.Done():
				return judge.Result{}, ctx.Err()
			}
		}
		reason = "stuck"
		select {
		case b.seen <- struct{}{}:
		default:
		}
	}
	if b.settled != nil && c.Action == nil && c.LastAction != nil && c.LastAction.Status == "aborted" {
		select {
		case b.settled <- struct{}{}:
		default:
		}
	}
	confidence := 1.0
	return judge.Result{Answers: map[string]judge.Answer{judge.InterruptQuestion: {Choice: reason, Confidence: &confidence}}}, nil
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
	for _, stalled := range []bool{false, true} {
		t.Run(fmt.Sprintf("stalled=%t", stalled), func(t *testing.T) {
			fb := &fakeBody{health: 20}
			if stalled {
				fb.holdFirst = true
			}
			p := &scripted{next: func(n int, _ harness.ChatParams) *harness.ChatResult {
				if stalled && n == 0 {
					return toolCall(n, "act", `{"type":"goto","x":10,"y":64,"z":0,"wait":60}`)
				}
				return toolCall(n, "observe", `{}`)
			}}
			o := setup(t, fb, p)
			o.MaxSteps = 3
			if stalled {
				cfg := judge.DefaultConfig()
				cfg.Rate = 100
				o.Judge = judge.New(judge.BodySource{URL: o.Body.URL}, actionStuckBackend{seen: make(chan struct{}, 1)}, nil, cfg)
			}

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
		})
	}
}

func TestActStopHonorsConditionalOwnership(t *testing.T) {
	fb := &fakeBody{health: 20, holdFirst: true}
	srv := httptest.NewServer(fb)
	defer srv.Close()
	client := &body.Client{URL: srv.URL}
	tb := &Toolbox{Body: client}
	var act Tool
	for _, tool := range tb.Tools() {
		if tool.Name == "act" {
			act = tool
		}
	}
	ctx := context.Background()
	if _, err := act.Run(ctx, json.RawMessage(`{"type":"goto","x":10,"y":64,"z":0,"wait":0}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := act.Run(ctx, json.RawMessage(`{"type":"stop","token":"earlier-incarnation"}`)); err != nil {
		t.Fatal(err)
	}
	card, err := client.State(ctx)
	if err != nil || card.Action == nil || card.Action.Token != "owner-1" || card.Action.Status != "running" {
		t.Fatalf("conditional mismatch stopped current owner: %+v err=%v", card.Action, err)
	}
	if _, err := act.Run(ctx, json.RawMessage(`{"type":"stop","token":"owner-1"}`)); err != nil {
		t.Fatal(err)
	}
	card, err = client.State(ctx)
	if err != nil || card.Action != nil || card.LastAction == nil || card.LastAction.Status != "aborted" {
		t.Fatalf("matching token did not stop owner: %+v err=%v", card, err)
	}
}

func TestBadToolArgumentsAreToolErrors(t *testing.T) {
	fb := &fakeBody{health: 20}
	bad := []struct{ tool, args, want string }{
		{"act", `{"type":"goto","x":"far","y":64,"z":0}`, "invalid goto action"},
		{"act", `{"type":"goto","x":1,"y":64}`, "field z in GotoAction: required"},
		{"act", `{"type":"fly"}`, "type must be one of"},
		{"act", `{"type":"collect","block":"oak_log","count":99}`, "count"},
		{"act", `{"type":"stop","token":""}`, "invalid stop action"},
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
