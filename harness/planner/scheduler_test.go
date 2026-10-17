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
	s.Execute = func(ctx context.Context, g goal.Goal) (executor.Outcome, error) {
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
	s := &planner.Scheduler{Memory: memoryStore, World: w, Plan: m.Plan, PollInterval: 5 * time.Millisecond, Check: func(context.Context, goal.Goal) (bool, error) { return false, nil }, Execute: func(context.Context, goal.Goal) (executor.Outcome, error) {
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
	s.Execute = func(_ context.Context, g goal.Goal) (executor.Outcome, error) {
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

func TestFailureAndDeathPostMortemsDoNotRetryLoop(t *testing.T) {
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
				if in.Event == event {
					// Sustained danger must not cancel its own post-mortem while
					// it is waiting for a real (slow) model response.
					select {
					case <-ctx.Done():
						return planner.Decision{}, ctx.Err()
					case <-time.After(25 * time.Millisecond):
					}
					return planner.Decision{Goal: target("must not retry", 99), Notes: []planner.Note{{Name: "lesson", Markdown: "Do not repeat the failed attempt.", Episodes: []int64{in.Episodes[0].ID}}}}, nil
				}
				return planner.Decision{Idle: true}, nil
			}
			s.Execute = func(context.Context, goal.Goal) (executor.Outcome, error) {
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
			before := plans.Load()
			time.Sleep(60 * time.Millisecond)
			stop()
			if plans.Load() != before || runs.Load() > 1 {
				t.Fatalf("retry loop plans=%d before=%d runs=%d", plans.Load(), before, runs.Load())
			}
			if event != "goal_failed" && runs.Load() != 0 {
				t.Fatal("danger/death triggered execution")
			}
		})
	}
}
