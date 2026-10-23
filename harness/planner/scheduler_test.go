package planner_test

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	harness "github.com/lox/agent-harness"
	"github.com/lox/clyde/harness/body"
	"github.com/lox/clyde/harness/executor"
	"github.com/lox/clyde/harness/goal"
	"github.com/lox/clyde/harness/judge"
	"github.com/lox/clyde/harness/memory"
	"github.com/lox/clyde/harness/planner"
)

type world struct {
	mu      sync.Mutex
	card    body.Card
	events  []body.Event
	replies []string
	owners  int
	overlap bool
	onReply func()
}

func newWorld() *world {
	w := &world{}
	w.card.Ts = int(time.Now().UnixMilli())
	w.card.Bot.Name = "Clyde"
	w.card.Bot.Dimension = "overworld"
	w.card.Bot.Health = 20
	w.card.Bot.Oxygen = 20
	w.card.Bot.Position = body.Vec{X: 17, Y: 73, Z: -6}
	return w
}
func (w *world) State(context.Context) (body.Card, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.card, nil
}
func (w *world) Events(context.Context, int) (body.EventPage, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	latest := 0
	if len(w.events) > 0 {
		latest = w.events[len(w.events)-1].Seq
	}
	return body.EventPage{Events: append([]body.Event(nil), w.events...), LatestSeq: latest}, nil
}
func (w *world) Stop(context.Context) error { return nil }
func (w *world) Action(_ context.Context, req any) (*body.ActionStatus, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.owners != 0 {
		w.overlap = true
	}
	if w.onReply != nil {
		w.onReply()
	}
	w.replies = append(w.replies, req.(map[string]any)["message"].(string))
	return &body.ActionStatus{}, nil
}
func (w *world) event(kind, user, msg string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, body.Event{Seq: len(w.events) + 1, TS: time.Now().UnixMilli(), Type: kind, Text: msg, Data: map[string]any{"username": user, "message": msg}})
}
func (w *world) replyCount() int { w.mu.Lock(); defer w.mu.Unlock(); return len(w.replies) }
func (w *world) enter() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.owners++
	if w.owners != 1 {
		w.overlap = true
	}
}
func (w *world) leave() { w.mu.Lock(); defer w.mu.Unlock(); w.owners-- }
func testStore(t *testing.T) *memory.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := memory.Open(filepath.Join(dir, "episodes.db"), filepath.Join(dir, "notes"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func start(t *testing.T, s *planner.Scheduler) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(5 * time.Second):
				t.Error("scheduler did not stop")
			}
		})
	}
	t.Cleanup(stop)
	return stop
}
func eventually(t *testing.T, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for !f() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
func await(t *testing.T, ch <-chan string, want string) {
	t.Helper()
	select {
	case got := <-ch:
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	case <-time.After(4 * time.Second):
		t.Fatalf("waiting for %s", want)
	}
}
func target(name string, n int) *goal.Goal {
	return &goal.Goal{Description: name, Until: goal.Predicate{Items: &goal.ItemsPredicate{Item: "minecraft:oak_log", Count: n}}}
}

func TestPlayerPreemptionWaitsForCleanupAndResumesAutonomous(t *testing.T) {
	w := newWorld()
	s := &planner.Scheduler{Memory: testStore(t), World: w, PollInterval: 5 * time.Millisecond, Check: func(context.Context, goal.Goal) (bool, error) { return false, nil }}
	s.Plan = func(_ context.Context, in planner.Input) (planner.Decision, error) {
		if in.Event == "idle" {
			return planner.Decision{Goal: target("autonomous", 7)}, nil
		}
		if in.Event == "player_request" {
			return planner.Decision{Goal: target("player", 3), Reply: "On it."}, nil
		}
		return planner.Decision{Idle: true}, nil
	}
	started := make(chan string, 8)
	cleaning := make(chan string, 1)
	cleanup := make(chan struct{})
	var runs atomic.Int32
	s.Execute = func(ctx context.Context, g goal.Goal, _ executor.ExecutionContext) (executor.Outcome, error) {
		w.enter()
		defer w.leave()
		started <- g.Description
		if runs.Add(1) == 1 {
			<-ctx.Done()
			cleaning <- "cleanup"
			<-cleanup
			return executor.Outcome{Outcome: executor.Cancelled}, nil
		}
		return executor.Outcome{Outcome: executor.Succeeded}, nil
	}
	stop := start(t, s)
	await(t, started, "autonomous")
	w.event("chat", "Alex", "Clyde: collect three logs")
	await(t, cleaning, "cleanup")
	time.Sleep(30 * time.Millisecond)
	if w.replyCount() != 0 || runs.Load() != 1 {
		t.Fatal("player work started before old owner finished cleanup")
	}
	close(cleanup)
	await(t, started, "player")
	await(t, started, "autonomous")
	eventually(t, func() bool {
		var c planner.Checkpoint
		return s.Memory.Load(context.Background(), &c) == nil && len(c.Jobs) == 0
	})
	stop()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.overlap || w.owners != 0 || len(w.replies) != 1 {
		t.Fatalf("owners=%d overlap=%v replies=%v", w.owners, w.overlap, w.replies)
	}
}

func TestSlowAutonomousPlanningCancelledAndChatDeduplicatedAfterRestart(t *testing.T) {
	w := newWorld()
	dir := t.TempDir()
	dbPath, notesPath := filepath.Join(dir, "episodes.db"), filepath.Join(dir, "notes")
	memoryStore, err := memory.Open(dbPath, notesPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { memoryStore.Close() })
	entered, cancelled := make(chan string, 1), make(chan string, 1)
	var calls atomic.Int32
	m := model(t, providerFunc(func(ctx context.Context, p harness.ChatParams) (*harness.ChatResult, error) {
		if calls.Add(1) == 1 {
			entered <- "planning"
			<-ctx.Done()
			cancelled <- "cancelled"
			return nil, ctx.Err()
		}
		return answer(`{"idle":true,"reply":"I cannot teleport players."}`)
	}))
	s := &planner.Scheduler{Memory: memoryStore, World: w, Plan: m.Plan, PollInterval: 5 * time.Millisecond, Check: func(context.Context, goal.Goal) (bool, error) { return false, nil }, Execute: func(context.Context, goal.Goal, executor.ExecutionContext) (executor.Outcome, error) {
		t.Error("unexpected executor")
		return executor.Outcome{}, nil
	}}
	stop := start(t, s)
	await(t, entered, "planning")
	w.event("chat", "Clyde", "Clyde own reply must be ignored")
	w.event("chat", "Alex", "ordinary conversation")
	w.event("whisper", "Alex", "teleport me to the moon")
	await(t, cancelled, "cancelled")
	eventually(t, func() bool { return w.replyCount() == 1 && calls.Load() == 3 })
	time.Sleep(40 * time.Millisecond)
	if calls.Load() != 3 {
		t.Fatal("idle observations caused model polling")
	}
	stop()
	if err := memoryStore.Close(); err != nil {
		t.Fatal(err)
	}
	memoryStore, err = memory.Open(dbPath, notesPath)
	if err != nil {
		t.Fatal(err)
	}
	s.Memory = memoryStore
	stop2 := start(t, s)
	time.Sleep(50 * time.Millisecond)
	stop2()
	if w.replyCount() != 1 || calls.Load() != 3 {
		t.Fatalf("replayed answered chat: replies=%d calls=%d", w.replyCount(), calls.Load())
	}
	// A new body may reuse sequence numbers, even passing the previous cursor.
	w.mu.Lock()
	w.events = nil
	w.mu.Unlock()
	time.Sleep(2 * time.Millisecond)
	w.event("whisper", "Alex", "new request after body restart")
	stop3 := start(t, s)
	eventually(t, func() bool { return w.replyCount() == 2 })
	stop3()
}

func TestRestartReconcilesLivePredicatesBeforeResuming(t *testing.T) {
	w := newWorld()
	store := testStore(t)
	state := planner.Checkpoint{Started: true, Jobs: []planner.Job{
		{ID: "held", Lane: "player", Event: "player_request", Status: "running", ReplySent: true, Goal: target("held", 3)},
		{ID: "unfinished", Lane: "autonomous", Event: "idle", Status: "running", Goal: target("unfinished", 7)},
	}}
	if err := store.Commit(context.Background(), state, nil); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var checked, executed []string
	s := &planner.Scheduler{Memory: store, World: w, PollInterval: 5 * time.Millisecond, Plan: func(context.Context, planner.Input) (planner.Decision, error) {
		return planner.Decision{Idle: true}, nil
	}}
	s.Check = func(_ context.Context, g goal.Goal) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		checked = append(checked, g.Description)
		return g.Description == "held", nil
	}
	s.Execute = func(_ context.Context, g goal.Goal, _ executor.ExecutionContext) (executor.Outcome, error) {
		mu.Lock()
		defer mu.Unlock()
		executed = append(executed, g.Description)
		return executor.Outcome{Outcome: executor.Succeeded}, nil
	}
	stop := start(t, s)
	eventually(t, func() bool {
		var c planner.Checkpoint
		return store.Load(context.Background(), &c) == nil && len(c.Jobs) == 0
	})
	stop()
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(checked, []string{"held", "unfinished"}) || !reflect.DeepEqual(executed, []string{"unfinished"}) || w.replyCount() != 0 {
		t.Fatalf("checked=%v executed=%v replies=%d", checked, executed, w.replyCount())
	}
}

func TestRefusalIsTerminalBeforeChatSend(t *testing.T) {
	w, store := newWorld(), testStore(t)
	if err := store.Commit(context.Background(), planner.Checkpoint{Started: true}, nil); err != nil {
		t.Fatal(err)
	}
	snapshot := make(chan planner.Checkpoint, 1)
	w.onReply = func() {
		var checkpoint planner.Checkpoint
		if err := store.Load(context.Background(), &checkpoint); err != nil {
			t.Error(err)
		}
		snapshot <- checkpoint
	}
	w.event("whisper", "Alex", "teleport me to the moon")
	s := &planner.Scheduler{Memory: store, World: w, PollInterval: 5 * time.Millisecond,
		Plan: func(context.Context, planner.Input) (planner.Decision, error) {
			return planner.Decision{Idle: true, Reply: "I cannot teleport players."}, nil
		},
		Execute: func(context.Context, goal.Goal, executor.ExecutionContext) (executor.Outcome, error) {
			t.Error("refusal dispatched execution")
			return executor.Outcome{}, nil
		},
		Check: func(context.Context, goal.Goal) (bool, error) { return false, nil },
	}
	stop := start(t, s)
	select {
	case checkpoint := <-snapshot:
		// This is exactly the durable state a restart sees if the process dies
		// after sending chat but before it can commit any later transition.
		if len(checkpoint.Jobs) != 0 {
			t.Fatalf("answered refusal remains eligible for replanning: %+v", checkpoint.Jobs)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("no reply")
	}
	stop()
}

func TestRecoveryAllowancePersistsWithoutNotificationRefill(t *testing.T) {
	for _, event := range []string{"goal_failed", "death", "judge_interrupt"} {
		t.Run(event, func(t *testing.T) {
			w := newWorld()
			store := testStore(t)
			var plans, runs atomic.Int32
			s := &planner.Scheduler{Memory: store, World: w, PollInterval: 5 * time.Millisecond, Check: func(context.Context, goal.Goal) (bool, error) { return false, nil }}
			s.Plan = func(ctx context.Context, in planner.Input) (planner.Decision, error) {
				plans.Add(1)
				if in.Event == "idle" && event == "goal_failed" {
					return planner.Decision{Goal: target("impossible", 99)}, nil
				}
				if in.Context.Mode != "" {
					// Sustained danger must not cancel its own post-mortem while
					// it is waiting for a real (slow) model response.
					select {
					case <-ctx.Done():
						return planner.Decision{}, ctx.Err()
					case <-time.After(25 * time.Millisecond):
					}
					return planner.Decision{Goal: target("changed approach", 99), Notes: []planner.Note{{Name: "lesson", Markdown: "Do not repeat the failed attempt.", Episodes: []int64{in.Episodes[0].ID}}}}, nil
				}
				return planner.Decision{Idle: true}, nil
			}
			s.Execute = func(context.Context, goal.Goal, executor.ExecutionContext) (executor.Outcome, error) {
				runs.Add(1)
				return executor.Outcome{Outcome: executor.Failed, Detail: "impossible"}, nil
			}
			if event == "death" {
				w.event("death", "", "died")
			}
			if event == "judge_interrupt" {
				w.mu.Lock()
				w.card.Bot.Health = 2
				w.mu.Unlock()
			}
			stop := start(t, s)
			eventually(t, func() bool { v, err := store.Semantic(); return err == nil && strings.Contains(v, "Do not repeat") })
			eventually(t, func() bool {
				var c planner.Checkpoint
				return store.Load(context.Background(), &c) == nil && c.Recovery != nil && c.Recovery.Closed && c.Recovery.Attempts == 2
			})
			before := plans.Load()
			time.Sleep(60 * time.Millisecond)
			stop()
			wantRuns := int32(2)
			if event == "goal_failed" {
				wantRuns++
			}
			if plans.Load() != before || runs.Load() != wantRuns {
				t.Fatalf("retry loop plans=%d before=%d runs=%d", plans.Load(), before, runs.Load())
			}
			// Restart and fresh danger/death notifications cannot refill a closed
			// episode or trigger another planning loop.
			w.mu.Lock()
			w.card.Bot.Health = 20
			w.mu.Unlock()
			stop2 := start(t, s)
			time.Sleep(30 * time.Millisecond)
			w.mu.Lock()
			w.card.Bot.Health = 2
			w.mu.Unlock()
			w.event("death", "", "another death")
			time.Sleep(50 * time.Millisecond)
			stop2()
			if plans.Load() != before || runs.Load() != wantRuns {
				t.Fatalf("restart/notification refilled allowance: plans=%d runs=%d", plans.Load(), runs.Load())
			}
		})
	}
}

func TestRecoveryCanChooseIdleOrChangedApproach(t *testing.T) {
	for _, tc := range []struct {
		name, mode string
		idle       bool
	}{
		{"idle", "recovery", true},
		{"changed approach", "recovery", false},
		{"model-only danger", "danger", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, store := newWorld(), testStore(t)
			var runs, plans atomic.Int32
			s := &planner.Scheduler{Memory: store, World: w, PollInterval: 5 * time.Millisecond, Check: func(context.Context, goal.Goal) (bool, error) { return false, nil }}
			s.Plan = func(_ context.Context, in planner.Input) (planner.Decision, error) {
				plans.Add(1)
				if in.Event == "idle" {
					return planner.Decision{Goal: target("original", 99)}, nil
				}
				if in.Context.Mode == tc.mode {
					if in.Context.Remaining != 2 {
						t.Errorf("initial recovery remaining=%d", in.Context.Remaining)
					}
					if tc.idle {
						return planner.Decision{Idle: true}, nil
					}
					return planner.Decision{Goal: target("changed", 4)}, nil
				}
				return planner.Decision{Idle: true}, nil
			}
			s.Execute = func(_ context.Context, g goal.Goal, intent executor.ExecutionContext) (executor.Outcome, error) {
				runs.Add(1)
				if g.Description == "original" {
					if tc.mode == "danger" {
						return executor.Outcome{Outcome: executor.Interrupted, Interrupt: &judge.Interrupt{Reason: "danger", Source: judge.SourceModel}}, nil
					}
					return executor.Outcome{Outcome: executor.Failed}, nil
				}
				if g.Description != "changed" || intent.Mode != tc.mode || intent.Remaining != 1 {
					t.Errorf("unexpected recovery %v %+v", g, intent)
				}
				return executor.Outcome{Outcome: executor.Succeeded}, nil
			}
			stop := start(t, s)
			eventually(t, func() bool {
				var c planner.Checkpoint
				wantPlans := int32(3)
				if tc.idle {
					wantPlans = 2
				}
				return store.Load(context.Background(), &c) == nil && c.Started && plans.Load() == wantPlans && len(c.Jobs) == 0
			})
			time.Sleep(40 * time.Millisecond)
			stop()
			var c planner.Checkpoint
			if err := store.Load(context.Background(), &c); err != nil {
				t.Fatal(err)
			}
			if tc.idle {
				if runs.Load() != 1 || plans.Load() != 2 || c.Recovery == nil || !c.Recovery.Closed || c.Recovery.Attempts != 0 {
					t.Fatalf("idle loop/refill runs=%d plans=%d state=%+v", runs.Load(), plans.Load(), c)
				}
			} else if runs.Load() != 2 || plans.Load() != 3 || c.Recovery != nil || c.AutoPaused {
				t.Fatalf("changed recovery/reset runs=%d plans=%d state=%+v", runs.Load(), plans.Load(), c)
			}
		})
	}
}

func TestRecoveryPreemptionPreservesChargedAllowance(t *testing.T) {
	w, store := newWorld(), testStore(t)
	cp := planner.Checkpoint{Started: true, AutoPaused: true, Recovery: &planner.RecoveryEpisode{Attempts: 1}, Jobs: []planner.Job{{ID: "recovery", Lane: "autonomous", Event: "goal_failed", PostMortem: true, Status: "queued", Goal: target("changed approach", 99), Context: executor.ExecutionContext{Mode: "recovery"}}}}
	if err := store.Commit(context.Background(), cp, nil); err != nil {
		t.Fatal(err)
	}
	started, cancelled := make(chan string, 1), make(chan string, 1)
	var runs atomic.Int32
	s := &planner.Scheduler{Memory: store, World: w, PollInterval: 5 * time.Millisecond,
		Check: func(context.Context, goal.Goal) (bool, error) { return false, nil },
		Plan: func(context.Context, planner.Input) (planner.Decision, error) {
			return planner.Decision{Idle: true}, nil
		},
		Execute: func(ctx context.Context, _ goal.Goal, intent executor.ExecutionContext) (executor.Outcome, error) {
			runs.Add(1)
			if intent.Mode != "recovery" || intent.Remaining != 0 {
				t.Errorf("lost recovery context: %+v", intent)
			}
			w.enter()
			defer w.leave()
			started <- "recovery"
			<-ctx.Done()
			cancelled <- "cleanup"
			return executor.Outcome{Outcome: executor.Cancelled}, nil
		},
	}
	stop := start(t, s)
	await(t, started, "recovery")
	w.mu.Lock()
	w.card.Bot.Health = 2
	w.mu.Unlock()
	eventually(t, func() bool {
		var c planner.Checkpoint
		return store.Load(context.Background(), &c) == nil && c.Danger
	})
	select {
	case <-cancelled:
		t.Fatal("new danger deterministically cancelled its own recovery")
	case <-time.After(100 * time.Millisecond):
	}
	w.event("whisper", "Alex", "please stop and reconsider")
	await(t, cancelled, "cleanup")
	eventually(t, func() bool {
		var c planner.Checkpoint
		return store.Load(context.Background(), &c) == nil && c.Recovery != nil && c.Recovery.Closed && c.Recovery.Attempts == 2 && len(c.Jobs) == 0
	})
	stop()
	if runs.Load() != 1 || w.overlap || w.owners != 0 {
		t.Fatalf("preemption refilled/laundered recovery or overlapped cleanup: runs=%d overlap=%v owners=%d", runs.Load(), w.overlap, w.owners)
	}
}

func TestRestartChargesUnfinishedRecoveryAndSafePlayerSuccessResets(t *testing.T) {
	for _, attempts := range []int{1, 2} {
		t.Run(string(rune('0'+attempts)), func(t *testing.T) {
			w, store := newWorld(), testStore(t)
			cp := planner.Checkpoint{Started: true, AutoPaused: true, Recovery: &planner.RecoveryEpisode{Attempts: attempts}, Jobs: []planner.Job{{ID: "crashed", Lane: "autonomous", Event: "goal_failed", PostMortem: true, Status: "running", Goal: target("crashed", 99), Context: executor.ExecutionContext{Mode: "recovery"}}}}
			if err := store.Commit(context.Background(), cp, nil); err != nil {
				t.Fatal(err)
			}
			var runs, plans atomic.Int32
			s := &planner.Scheduler{Memory: store, World: w, PollInterval: 5 * time.Millisecond, Check: func(context.Context, goal.Goal) (bool, error) { return false, nil }}
			s.Plan = func(_ context.Context, in planner.Input) (planner.Decision, error) {
				plans.Add(1)
				if in.Event == "goal_finished" {
					return planner.Decision{Idle: true}, nil
				}
				return planner.Decision{Goal: target("new approach", 4)}, nil
			}
			s.Execute = func(_ context.Context, g goal.Goal, intent executor.ExecutionContext) (executor.Outcome, error) {
				runs.Add(1)
				if g.Description == "crashed" {
					t.Error("replayed charged attempt")
				}
				if intent.Mode == "" {
					return executor.Outcome{Outcome: executor.Succeeded}, nil
				}
				return executor.Outcome{Outcome: executor.Failed}, nil
			}
			stop := start(t, s)
			eventually(t, func() bool {
				var c planner.Checkpoint
				return store.Load(context.Background(), &c) == nil && c.Recovery != nil && c.Recovery.Closed && c.Recovery.Attempts == 2
			})
			if runs.Load() != int32(2-attempts) || plans.Load() != int32(2-attempts) {
				t.Fatalf("allowance replay/refill runs=%d plans=%d", runs.Load(), plans.Load())
			}
			w.event("whisper", "Alex", "make a useful safe item")
			eventually(t, func() bool {
				var c planner.Checkpoint
				return store.Load(context.Background(), &c) == nil && c.Recovery == nil && len(c.Jobs) == 0
			})
			stop()
			if runs.Load() != int32(3-attempts) {
				t.Fatalf("safe player work did not execute: %d", runs.Load())
			}
		})
	}
}
