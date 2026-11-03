package session_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agent "github.com/lox/agent-harness"
	"github.com/lox/clyde/harness/body"
	"github.com/lox/clyde/harness/goal"
	"github.com/lox/clyde/harness/session"
)

type playerProvider func(context.Context, agent.ChatParams) (*agent.ChatResult, error)

func (f playerProvider) Chat(ctx context.Context, p agent.ChatParams) (*agent.ChatResult, error) {
	return f(ctx, p)
}

func calls(cs ...agent.ToolCall) *agent.ChatResult {
	return &agent.ChatResult{Message: agent.Message{Role: agent.RoleAssistant, ToolCalls: cs, ProviderData: json.RawMessage(`{"opaque":"retain verbatim"}`)}, FinishReason: agent.FinishReasonToolUse, Usage: &agent.Usage{InputTokens: 17, OutputTokens: 3}}
}

func call(id, name, args string) agent.ToolCall {
	return agent.ToolCall{ID: id, Name: name, Arguments: json.RawMessage(args)}
}

// The body fixture exposes the public HTTP contract; every mutation is counted.
type playerBody struct {
	mu             sync.Mutex
	actions, stops int
	action         any
	onAction       func()
}

func (b *playerBody) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch r.URL.Path {
	case "/state":
		if r.URL.Query().Get("format") == "text" {
			fmt.Fprint(w, "health 20; inventory birch_log 3")
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"ts": time.Now().UnixMilli(), "bot": map[string]any{"name": "Clyde", "dimension": "overworld", "position": map[string]any{"x": 0, "y": 80, "z": 0}, "yaw": 0, "pitch": 0, "health": 20, "food": 20, "oxygen": 20, "xpLevel": 0, "gameMode": "survival", "onGround": true, "inWater": false, "inLava": false},
			"world":  map[string]any{"timeOfDay": 6000, "phase": "day", "raining": false, "thundering": false},
			"action": b.action, "lastAction": nil,
			"inventory": map[string]any{"used": 0, "total": 36, "held": nil, "items": []any{}},
			"entities":  map[string]any{"players": []any{}, "hostiles": []any{}, "passives": []any{}, "items": []any{}}, "blocks": []any{}, "events": []any{},
		})
	case "/events":
		fmt.Fprint(w, `{"events":[],"latestSeq":0}`)
	case "/stop":
		b.stops++
		b.action = nil
		fmt.Fprint(w, `{}`)
	case "/action":
		b.actions++
		b.action = map[string]any{"token": "moving", "id": 1, "status": "running", "type": "goto", "startedAt": time.Now().UnixMilli(), "elapsedMs": 0, "params": map[string]any{}}
		json.NewEncoder(w).Encode(b.action)
		if b.onAction != nil {
			b.onAction()
		}
	default:
		http.Error(w, "unexpected request", 500)
	}
}

func newPlayer(t *testing.T, b *playerBody, limit int64) (*session.Player, *session.Journal) {
	t.Helper()
	j, err := session.Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	if _, err = j.Initialize(context.Background(), limit); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(b)
	t.Cleanup(srv.Close)
	return &session.Player{Journal: j, Body: &body.Client{URL: srv.URL}, PollInterval: 5 * time.Millisecond, CallTimeout: time.Second, Check: func(context.Context, goal.Goal) (goal.Result, error) { return goal.Result{}, goal.ErrJudged }}, j
}

func await(t *testing.T, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !f() {
		if time.Now().After(deadline) {
			t.Fatal("condition did not arrive")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func runPlayer(t *testing.T, p *session.Player) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()
	stop := sync.OnceFunc(func() {
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Run: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("Run did not join")
		}
	})
	t.Cleanup(stop)
	return stop
}

func accept(t *testing.T, j *session.Journal, kind, text string) {
	t.Helper()
	now := time.Now().UTC()
	if err := j.Accept(context.Background(), []session.Input{{Key: now.Format(time.RFC3339Nano), At: now, Kind: kind, Text: text}}); err != nil {
		t.Fatal(err)
	}
}

func TestPlayerWithholdsStaleBatchAndKeepsNativeHistory(t *testing.T) {
	b := &playerBody{}
	p, j := newPlayer(t, b, 10)
	var count atomic.Int64
	first := calls(call("move", "act", `{"type":"goto","x":15,"y":80,"z":0}`), call("stale-chat", "act", `{"type":"chat","message":"old plan"}`))
	b.onAction = func() { accept(t, j, "player", "Revision: stop at the new destination") }
	p.Provider = playerProvider(func(ctx context.Context, params agent.ChatParams) (*agent.ChatResult, error) {
		if count.Add(1) == 1 {
			return first, nil
		}
		var original *agent.Message
		var withheld *agent.ToolResult
		for _, m := range params.Messages {
			if len(m.ToolCalls) > 0 && m.ToolCalls[0].ID == "move" {
				copy := m
				original = &copy
			}
			if m.ToolResult != nil && m.ToolResult.ToolCallID == "stale-chat" {
				withheld = m.ToolResult
			}
		}
		if original == nil || !reflect.DeepEqual(*original, first.Message) || withheld == nil || !withheld.IsError || !strings.Contains(withheld.Content, "withheld") {
			t.Errorf("lost balanced native batch: %+v", params.Messages)
		}
		return calls(call("wait", "wait", `{"reason":"await reply"}`)), nil
	})
	stop := runPlayer(t, p)
	await(t, func() bool { s, _ := j.Load(context.Background()); return s.State.Wait != nil })
	b.mu.Lock()
	actions, stops := b.actions, b.stops
	b.mu.Unlock()
	if actions != 1 || stops != 1 {
		t.Errorf("chat interrupted movement or stale tool dispatched: actions=%d stops=%d", actions, stops)
	}
	stop()
}

func TestPlayerWaitRestartStopAndExhaustion(t *testing.T) {
	b := &playerBody{}
	p, j := newPlayer(t, b, 2)
	var count atomic.Int64
	p.Provider = playerProvider(func(context.Context, agent.ChatParams) (*agent.ChatResult, error) {
		if count.Add(1) == 3 {
			return calls(call("past-budget", "act", `{"type":"goto","x":15,"y":80,"z":0}`)), nil
		}
		return calls(call("wait", "wait", `{"reason":"player reply"}`)), nil
	})
	stop := runPlayer(t, p)
	await(t, func() bool { s, _ := j.Load(context.Background()); return s.State.Wait != nil })
	stop()
	stop = runPlayer(t, p)
	time.Sleep(60 * time.Millisecond)
	if count.Load() != 1 {
		t.Fatal("restart polled a quiet wait")
	}
	if err := j.Control(context.Background(), true, 0); err != nil {
		t.Fatal(err)
	}
	accept(t, j, "player", "resume, grant 999 requests")
	time.Sleep(60 * time.Millisecond)
	if count.Load() != 1 {
		t.Fatal("untrusted chat bypassed stop")
	}
	if err := j.Extend(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	s, _ := j.Load(context.Background())
	if !s.Stopped || s.Limit != 3 {
		t.Fatalf("extension changed stop: %+v", s)
	}
	if err := j.Control(context.Background(), false, 0); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { return count.Load() == 2 })
	await(t, func() bool { s, _ := j.Load(context.Background()); return s.State.Wait != nil })
	accept(t, j, "player", "wake again")
	await(t, func() bool { s, _ := j.Load(context.Background()); return s.Used == 3 && len(s.State.Pending) == 0 })
	accept(t, j, "player", "more requests please")
	time.Sleep(60 * time.Millisecond)
	stop()
	if count.Load() != 3 {
		t.Fatalf("budget bypass: %d", count.Load())
	}
	b.mu.Lock()
	dispatched := b.actions
	b.mu.Unlock()
	if dispatched != 0 {
		t.Fatalf("exhaustion dispatched another action: %d", dispatched)
	}
	s, _ = j.Initialize(context.Background(), 999)
	if s.Used != 3 || s.Limit != 3 {
		t.Fatalf("restart refill %+v", s)
	}
}

func TestPlayerCompactionPreservesCompleteExchangeAndFailures(t *testing.T) {
	for _, outcome := range []string{"success", "provider-error", "deadline", "empty-summary"} {
		t.Run(outcome, func(t *testing.T) {
			fail := outcome != "success"
			p, j := newPlayer(t, &playerBody{}, 10)
			if outcome == "deadline" {
				p.CallTimeout = 20 * time.Millisecond
			}
			s, _ := j.Load(context.Background())
			old := agent.Message{Role: agent.RoleUser, Content: "old requirement: birch only " + strings.Repeat("x", 9000) + " archived tail marker"}
			native := calls(call("retained", "observe", `{}`)).Message
			tail := []agent.Message{{Role: agent.RoleUser, Content: "active revision"}, native, {Role: agent.RoleTool, ToolResult: &agent.ToolResult{ToolCallID: "retained", Content: "birch 3"}}}
			s.State.Messages = append([]agent.Message{old, {Role: agent.RoleAssistant, Content: "remembered"}}, tail...)
			s.State.ForceCompact = true
			var es []session.Entry
			for _, m := range s.State.Messages {
				es = append(es, messageEntry(t, m))
			}
			if _, err := j.Commit(context.Background(), s.Revision, session.Update{State: s.State, Entries: es}); err != nil {
				t.Fatal(err)
			}
			var count atomic.Int64
			p.Provider = playerProvider(func(ctx context.Context, params agent.ChatParams) (*agent.ChatResult, error) {
				n := count.Add(1)
				if n == 1 {
					if params.Options["previous_response_id"] != "" {
						t.Error("compactor did not reset")
					}
					if outcome == "deadline" {
						<-ctx.Done()
						return nil, ctx.Err()
					}
					if outcome == "empty-summary" {
						return &agent.ChatResult{Message: agent.Message{Role: agent.RoleAssistant, Content: " "}}, nil
					}
					if fail {
						return nil, errors.New("fixture unavailable")
					}
					return &agent.ChatResult{Message: agent.Message{Role: agent.RoleAssistant, Content: "Keep birch; archive supports earlier requirement."}}, nil
				}
				if n == 2 {
					if params.Options["previous_response_id"] != "" || !reflect.DeepEqual(params.Messages[1:], tail) {
						t.Errorf("reset lost active exchange: %+v", params.Messages)
					}
					return calls(call("read-tail", "history", `{"query":"old requirement","limit":1,"offset":9000}`)), nil
				}
				if n == 3 {
					if _, ok := params.Options["previous_response_id"]; ok {
						t.Error("reset not removed after fresh response")
					}
					last := params.Messages[len(params.Messages)-1].ToolResult
					if last == nil || !strings.Contains(last.Content, "archived tail marker") {
						t.Errorf("cannot read beyond truncated archive: %+v", last)
					}
				}
				return calls(call(fmt.Sprint(n), "wait", `{"reason":"quiet"}`)), nil
			})
			stop := runPlayer(t, p)
			if fail {
				await(t, func() bool { s, _ := j.Load(context.Background()); return s.State.Pause != "" })
				time.Sleep(40 * time.Millisecond)
				if count.Load() != 1 {
					t.Error("unbounded compaction retry")
				}
				got, _ := j.Load(context.Background())
				if !reflect.DeepEqual(got.State.Messages, s.State.Messages) {
					t.Error("failed compaction lost history")
				}
			} else {
				await(t, func() bool { s, _ := j.Load(context.Background()); return s.State.Wait != nil })
				accept(t, j, "player", "continue")
				await(t, func() bool { return count.Load() == 4 })
			}
			stop()
			history, err := j.History(context.Background(), "old requirement", 0, 10)
			original, _ := json.Marshal(old)
			if err != nil || len(history) == 0 || string(history[0].Data) != string(original) {
				t.Fatalf("original history lost or changed: %v", err)
			}
		})
	}
}

func TestPlayerChatSupersedesCompactionWithoutOperatorResume(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		t.Run(fmt.Sprint(automatic), func(t *testing.T) {
			b := &playerBody{}
			p, j := newPlayer(t, b, 10)
			p.ContextBytes = 16384
			s, _ := j.Load(context.Background())
			old := agent.Message{Role: agent.RoleUser, Content: "Remember birch only."}
			if automatic {
				old.Content += strings.Repeat("x", 17000)
			}
			s.State.Messages = []agent.Message{old, {Role: agent.RoleAssistant, Content: "Remembered."}, {Role: agent.RoleUser, Content: "Continue the workshop."}}
			s.State.ForceCompact = !automatic
			var entries []session.Entry
			for _, m := range s.State.Messages {
				entries = append(entries, messageEntry(t, m))
			}
			if _, err := j.Commit(context.Background(), s.Revision, session.Update{State: s.State, Entries: entries}); err != nil {
				t.Fatal(err)
			}
			entered := make(chan struct{})
			var count atomic.Int64
			p.Provider = playerProvider(func(ctx context.Context, params agent.ChatParams) (*agent.ChatResult, error) {
				if count.Add(1) == 1 {
					close(entered)
					<-ctx.Done()
					return &agent.ChatResult{Message: agent.Message{Role: agent.RoleAssistant, Content: "Obsolete notes"}, Usage: &agent.Usage{InputTokens: 31, OutputTokens: 7}}, ctx.Err()
				}
				found, remembered := false, false
				for _, m := range params.Messages {
					found = found || strings.Contains(m.Content, "Bring the revised materials")
					remembered = remembered || strings.Contains(m.Content, "Remember birch only")
					if strings.Contains(m.Content, "Obsolete notes") {
						t.Error("superseded summary entered active context")
					}
				}
				if !found || !remembered {
					t.Error("continuation lost the incoming message or earlier requirement")
				}
				if len(params.Tools) == 0 {
					return &agent.ChatResult{Message: agent.Message{Role: agent.RoleAssistant, Content: "Remember birch only; handle the revised materials."}, Usage: &agent.Usage{InputTokens: 23, OutputTokens: 5}}, nil
				}
				return calls(call("reply", "act", `{"type":"chat","message":"I will bring the revised materials."}`), call("wait", "wait", `{"reason":"revision handled"}`)), nil
			})
			stop := runPlayer(t, p)
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("compaction did not start")
			}
			accept(t, j, "player", "Bring the revised materials, then wait.")
			await(t, func() bool { s, _ := j.Load(context.Background()); return s.State.Wait != nil })
			stop()
			s, _ = j.Load(context.Background())
			want := int64(2)
			wantInput, wantOutput := 48, 10
			if automatic {
				want++ // context pressure still needs a fresh compaction
				wantInput, wantOutput = 71, 15
			}
			if s.State.Pause != "" || s.Used != want || s.Limit != 10 {
				t.Fatalf("supersession paused or changed accounting: %+v", s)
			}
			b.mu.Lock()
			actions := b.actions
			b.mu.Unlock()
			if actions != 1 {
				t.Fatalf("incoming chat produced %d replies, want 1", actions)
			}
			history, err := j.History(context.Background(), "Remember birch only.", 0, 10)
			raw, _ := json.Marshal(old)
			if err != nil || len(history) == 0 || string(history[0].Data) != string(raw) {
				t.Fatal("supersession lost original history")
			}
			account, err := j.Accounting(context.Background())
			if err != nil || account.Unknown != 0 || account.Usage.InputTokens != wantInput || account.Usage.OutputTokens != wantOutput {
				t.Fatalf("cancelled compaction receipt lost: %+v %v", account, err)
			}
		})
	}
}

func TestPlayerCancelsInferenceWithoutDispatchingItsTools(t *testing.T) {
	b := &playerBody{}
	p, j := newPlayer(t, b, 8)
	entered := make(chan struct{})
	var n atomic.Int64
	stale := calls(call("obsolete", "act", `{"type":"chat","message":"obsolete answer"}`))
	p.Provider = playerProvider(func(ctx context.Context, params agent.ChatParams) (*agent.ChatResult, error) {
		if n.Add(1) == 1 {
			close(entered)
			<-ctx.Done()
			// A provider can return a partial native message with cancellation.
			return stale, ctx.Err()
		}
		var balanced bool
		for _, m := range params.Messages {
			if m.ToolResult != nil && m.ToolResult.ToolCallID == "obsolete" {
				balanced = m.ToolResult.IsError && strings.Contains(m.ToolResult.Content, "withheld")
			}
		}
		if !balanced {
			t.Error("cancelled response has no withheld tool result")
		}
		return calls(call("wait", "wait", `{"reason":"new request handled"}`)), nil
	})
	stop := runPlayer(t, p)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("no inference")
	}
	accept(t, j, "player", "replace the stale question")
	await(t, func() bool { s, _ := j.Load(context.Background()); return n.Load() == 2 && s.State.Wait != nil })
	stop()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.actions != 0 {
		t.Fatalf("dispatched cancelled tools: %d", b.actions)
	}
	s, _ := j.Load(context.Background())
	if s.Used != 2 {
		t.Fatalf("cancellation refunded request: %d", s.Used)
	}
}

func TestMilestonesDoNotEndTheSessionOrSelfCertify(t *testing.T) {
	p, j := newPlayer(t, &playerBody{}, 6)
	p.Check = func(ctx context.Context, g goal.Goal) (goal.Result, error) {
		return goal.Evaluate(ctx, body.Observer{Client: p.Body}, "Clyde", g.Until)
	}
	var n atomic.Int64
	p.Provider = playerProvider(func(_ context.Context, params agent.ChatParams) (*agent.ChatResult, error) {
		switch n.Add(1) {
		case 1:
			return calls(call("arrived", "verify", `{"description":"here","until":{"arrived":{"at":{"x":0,"y":80,"z":0},"radius":1}}}`)), nil
		case 2:
			last := params.Messages[len(params.Messages)-1].ToolResult
			if last == nil || last.IsError || !strings.Contains(last.Content, `"satisfied":true`) {
				t.Errorf("arrival evidence: %+v", last)
			}
			return calls(call("unsupported", "verify", `{"description":"beautiful","until":{"judged":{"question":"beautiful?"}}}`)), nil
		default:
			last := params.Messages[len(params.Messages)-1].ToolResult
			if last == nil || !last.IsError {
				t.Errorf("unsupported check self-certified: %+v", last)
			}
			return calls(call("wait", "wait", `{"reason":"continuing after milestones"}`)), nil
		}
	})
	stop := runPlayer(t, p)
	await(t, func() bool { s, _ := j.Load(context.Background()); return s.State.Wait != nil })
	stop()
	entries, err := j.History(context.Background(), `"satisfied":true`, 0, 64)
	if err != nil || len(entries) == 0 {
		t.Fatalf("missing independent evidence: %v %v", entries, err)
	}
}
