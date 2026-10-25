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
	// Chat is not an exclusive world-action owner.
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

func TestSafeFollowUpRetainsLegacyClosureAndOrdinaryAdmission(t *testing.T) {
	w := newWorld()
	store := testStore(t)
	legacy := planner.Checkpoint{Started: true, AutoPaused: true, Recovery: &planner.RecoveryEpisode{Closed: true}}
	if err := store.Commit(context.Background(), legacy, nil); err != nil {
		t.Fatal(err)
	}
	inputs := make(chan planner.Input, 2)
	executed := make(chan executor.ExecutionContext, 1)
	s := &planner.Scheduler{Memory: store, World: w, PollInterval: 5 * time.Millisecond,
		Check: func(context.Context, goal.Goal) (bool, error) { return false, nil },
		Plan: func(_ context.Context, in planner.Input) (planner.Decision, error) {
			inputs <- in
			return planner.Decision{Goal: target("safe follow-up", 8)}, nil
		},
		Execute: func(ctx context.Context, _ goal.Goal, c executor.ExecutionContext) (executor.Outcome, error) {
			executed <- c
			<-ctx.Done()
			return executor.Outcome{Outcome: executor.Cancelled}, nil
		},
	}
	stop := start(t, s)
	w.event("whisper", "Alex", "Clear the leaves around you and finish gathering the eight birch logs.")
	var in planner.Input
	select {
	case in = <-inputs:
	case <-time.After(time.Second):
		t.Fatal("follow-up not planned")
	}
	f := in.Context.Control
	if f == nil || !f.Eligible || f.Admission != "ordinary" || !f.AutoPaused || !f.Closed || f.Attempts != 0 || f.ClosureReason != "unknown" || f.PauseReason != "unknown" {
		t.Fatalf("legacy closure misrepresented: %+v", f)
	}
	select {
	case c := <-executed:
		if c.Mode != "" || c.Control == nil || !c.Control.Eligible || c.Control.Attempts != 0 || !c.Control.Closed || c.Control.ExecutorOwner == "" {
			t.Fatalf("safe work not admitted ordinarily: %+v", c)
		}
	case <-time.After(time.Second):
		t.Fatal("safe follow-up not dispatched")
	}
	stop()
	var got planner.Checkpoint
	if err := store.Load(context.Background(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Recovery == nil || got.Recovery.Attempts != 0 || !got.Recovery.Closed || !got.AutoPaused || got.Recovery.ClosureReason != "" {
		t.Fatalf("request refilled or relabelled legacy allowance: %+v", got)
	}
	// Restart must not periodically replan the closed episode or refill it.
	stop = start(t, s)
	time.Sleep(50 * time.Millisecond)
	stop()
	select {
	case extra := <-inputs:
		t.Fatalf("unexpected periodic planning: %+v", extra)
	default:
	}
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
	if runs.Load() != 1 {
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
		{ID: "obsolete", Lane: "player", Status: "running", Superseded: true, Goal: target("must not replay", 99)},
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
		if len(checkpoint.Dialogue) != 2 || !checkpoint.Dialogue[1].ReplyIntent || checkpoint.Dialogue[1].Text != "I cannot teleport players." || checkpoint.Dialogue[0].RequestID != checkpoint.Dialogue[1].RequestID {
			t.Fatalf("exact reply/identity not durable before send: %+v", checkpoint.Dialogue)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("no reply")
	}
	stop()
}

func TestConversationPreservesWorkUntilModelReplacesIt(t *testing.T) {
	w, store := newWorld(), testStore(t)
	initial := planner.Checkpoint{Started: true, Project: &planner.Project{Summary: "Alex prefers birch; workshop is ongoing", Request: "Alex: workshop"}, Jobs: []planner.Job{{ID: "travel", Lane: "player", Status: "queued", Goal: target("travel", 7)}}}
	if err := store.Commit(context.Background(), initial, nil); err != nil {
		t.Fatal(err)
	}
	started, cleaning := make(chan string, 3), make(chan string, 1)
	cleanup := make(chan struct{})
	var runs atomic.Int32
	s := &planner.Scheduler{Memory: store, World: w, PollInterval: 5 * time.Millisecond, Check: func(context.Context, goal.Goal) (bool, error) { return false, nil }}
	s.Execute = func(ctx context.Context, g goal.Goal, _ executor.ExecutionContext) (executor.Outcome, error) {
		w.enter()
		defer w.leave()
		runs.Add(1)
		started <- g.Description
		if g.Description == "travel" {
			<-ctx.Done()
			cleaning <- "cleanup"
			<-cleanup
			return executor.Outcome{Outcome: executor.Cancelled}, nil
		}
		return executor.Outcome{Outcome: executor.Succeeded}, nil
	}
	s.Plan = func(_ context.Context, in planner.Input) (planner.Decision, error) {
		if in.Event != "player_request" {
			return planner.Decision{Idle: true}, nil
		}
		if strings.HasPrefix(in.Request, "Alex:") {
			if len(in.Work) != 1 || in.Work[0].Goal.Description != "travel" || in.Project == nil {
				t.Errorf("missing work/project context: %+v", in)
			}
			return planner.Decision{Idle: true, Work: "preserve", Reply: "Which side of the workshop do you mean?"}, nil
		}
		if len(in.Dialogue) != 3 || in.Dialogue[0].Speaker != "Alex" || !in.Dialogue[1].ReplyIntent || in.Dialogue[2].Speaker != "Bob" || in.Dialogue[0].RequestID != in.Dialogue[1].RequestID {
			t.Errorf("dialogue lost identity/order: %+v", in.Dialogue)
		}
		project := "Bob revised the workshop location; still use birch."
		return planner.Decision{Goal: target("revised", 3), Work: "replace", Project: &project, Reply: "I will change the work."}, nil
	}
	stop := start(t, s)
	await(t, started, "travel")
	w.event("whisper", "Alex", "which side?")
	eventually(t, func() bool { return w.replyCount() == 1 })
	select {
	case <-cleaning:
		t.Fatal("clarification cancelled travel")
	case <-time.After(35 * time.Millisecond):
	}
	w.event("whisper", "Bob", "use the other location instead")
	await(t, cleaning, "cleanup")
	eventually(t, func() bool { return w.replyCount() == 2 })
	var replacing planner.Checkpoint
	if err := store.Load(context.Background(), &replacing); err != nil {
		t.Fatal(err)
	}
	for _, job := range replacing.Jobs {
		if job.ID == "travel" && !job.Superseded {
			t.Fatal("replacement cancellation intent was not durable before reply")
		}
	}
	if runs.Load() != 1 {
		t.Fatal("replacement overlapped cleanup")
	}
	close(cleanup)
	await(t, started, "revised")
	eventually(t, func() bool {
		var c planner.Checkpoint
		return store.Load(context.Background(), &c) == nil && len(c.Jobs) == 0
	})
	stop()
	var c planner.Checkpoint
	if err := store.Load(context.Background(), &c); err != nil {
		t.Fatal(err)
	}
	if runs.Load() != 2 || w.overlap || c.Project == nil || !strings.Contains(c.Project.Request, "Bob:") || !strings.Contains(c.Project.Summary, "birch") {
		t.Fatalf("lost replacement/ownership/project: runs=%d checkpoint=%+v", runs.Load(), c)
	}
}

func TestStaleConversationDropsAllMutationsAndRefreshesOnce(t *testing.T) {
	w, store := newWorld(), testStore(t)
	cp := planner.Checkpoint{Started: true, Project: &planner.Project{Summary: "original intent"}, Jobs: []planner.Job{{ID: "work", Lane: "player", Status: "queued", Goal: target("original", 1)}}}
	if err := store.Commit(context.Background(), cp, nil); err != nil {
		t.Fatal(err)
	}
	working, planning := make(chan string, 1), make(chan string, 1)
	complete, release := make(chan struct{}), make(chan struct{})
	var plans, runs atomic.Int32
	s := &planner.Scheduler{Memory: store, World: w, PollInterval: 5 * time.Millisecond, Check: func(context.Context, goal.Goal) (bool, error) { return false, nil }}
	s.Execute = func(ctx context.Context, _ goal.Goal, _ executor.ExecutionContext) (executor.Outcome, error) {
		runs.Add(1)
		working <- "work"
		select {
		case <-complete:
			return executor.Outcome{Outcome: executor.Succeeded}, nil
		case <-ctx.Done():
			return executor.Outcome{Outcome: executor.Cancelled}, nil
		}
	}
	s.Plan = func(_ context.Context, in planner.Input) (planner.Decision, error) {
		if in.Event != "player_request" {
			return planner.Decision{Idle: true}, nil
		}
		if plans.Add(1) == 1 {
			planning <- "planning"
			<-release // deliberately returns a late, otherwise valid decision
			bad := "stale project"
			return planner.Decision{Goal: target("stale", 99), Work: "replace", Project: &bad, Reply: "stale reply", Notes: []planner.Note{{Name: "stale", Markdown: "stale note", Episodes: []int64{in.Episodes[0].ID}}}}, nil
		}
		if len(in.Work) != 0 {
			t.Error("refresh did not see completed work")
		}
		return planner.Decision{Idle: true, Work: "preserve", Reply: "Fresh answer after completion."}, nil
	}
	stop := start(t, s)
	await(t, working, "work")
	w.event("whisper", "Alex", "status?")
	await(t, planning, "planning")
	close(complete)
	eventually(t, func() bool {
		es, e := store.Recent(context.Background(), 16)
		if e != nil {
			return false
		}
		for _, ep := range es {
			if ep.Kind == "goal_succeeded" {
				return true
			}
		}
		return false
	})
	close(release)
	eventually(t, func() bool { return w.replyCount() == 1 })
	stop()
	var got planner.Checkpoint
	store.Load(context.Background(), &got)
	semantic, err := store.Semantic()
	if err != nil {
		t.Fatal(err)
	}
	if plans.Load() != 2 || runs.Load() != 1 || got.Project.Summary != "original intent" || semantic != "" || w.replies[0] != "Fresh answer after completion." {
		t.Fatalf("stale decision leaked: plans=%d runs=%d project=%+v notes=%s replies=%v", plans.Load(), runs.Load(), got.Project, semantic, w.replies)
	}
}

func TestConversationDoesNotLaunderRecovery(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(map[bool]string{false: "running recovery", true: "exhausted"}[closed], func(t *testing.T) {
			w, store := newWorld(), testStore(t)
			w.card.Bot.Health = 2
			cp := planner.Checkpoint{Started: true, Danger: true, AutoPaused: true, Recovery: &planner.RecoveryEpisode{Attempts: 2, Closed: closed}}
			if !closed {
				cp.Recovery.Attempts = 1
				cp.Jobs = []planner.Job{{ID: "recovery", Lane: "autonomous", Status: "queued", PostMortem: true, Context: executor.ExecutionContext{Mode: "danger"}, Goal: target("recovery", 99)}}
			}
			if err := store.Commit(context.Background(), cp, nil); err != nil {
				t.Fatal(err)
			}
			started := make(chan string, 1)
			var plans atomic.Int32
			s := &planner.Scheduler{Memory: store, World: w, PollInterval: 5 * time.Millisecond, Check: func(context.Context, goal.Goal) (bool, error) { return false, nil }, Plan: func(context.Context, planner.Input) (planner.Decision, error) {
				plans.Add(1)
				return planner.Decision{Idle: true, Work: "preserve", Reply: "Still discussing; no change."}, nil
			}, Execute: func(ctx context.Context, _ goal.Goal, _ executor.ExecutionContext) (executor.Outcome, error) {
				started <- "recovery"
				<-ctx.Done()
				return executor.Outcome{Outcome: executor.Cancelled}, nil
			}}
			stop := start(t, s)
			if !closed {
				await(t, started, "recovery")
			}
			w.event("whisper", "Alex", "what is happening?")
			eventually(t, func() bool { return w.replyCount() == 1 })
			time.Sleep(35 * time.Millisecond)
			var got planner.Checkpoint
			if err := store.Load(context.Background(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Recovery == nil || got.Recovery.Attempts != 2 || got.Recovery.Closed != closed || !got.AutoPaused || plans.Load() != 1 {
				t.Fatalf("conversation changed recovery: %+v plans=%d", got, plans.Load())
			}
			stop()
		})
	}
}

func TestDialogueBoundAndProjectSurviveRestartWithoutReplayingIntents(t *testing.T) {
	w, store := newWorld(), testStore(t)
	var calls atomic.Int32
	s := &planner.Scheduler{Memory: store, World: w, PollInterval: 5 * time.Millisecond, Check: func(context.Context, goal.Goal) (bool, error) { return false, nil }, Execute: func(context.Context, goal.Goal, executor.ExecutionContext) (executor.Outcome, error) {
		t.Error("conversation started execution")
		return executor.Outcome{}, nil
	}}
	s.Plan = func(_ context.Context, in planner.Input) (planner.Decision, error) {
		if in.Event != "player_request" {
			return planner.Decision{Idle: true}, nil
		}
		calls.Add(1)
		if calls.Load() == 1 {
			project := "Alex agreed on birch; await supplies"
			return planner.Decision{Idle: true, Work: "preserve", Project: &project, Reply: "Which supplies?"}, nil
		}
		if len(in.Dialogue) > 12 || in.Project == nil || !strings.Contains(in.Project.Summary, "birch") {
			t.Errorf("lost bounded context/project: %+v", in)
		}
		if calls.Load() == 9 {
			for _, turn := range in.Dialogue {
				if strings.Contains(turn.Text, "prefer birch") {
					t.Error("old preference still in recent window")
				}
			}
		}
		return planner.Decision{Idle: true, Work: "preserve", Reply: "Agreed."}, nil
	}
	stop := start(t, s)
	w.event("whisper", "Alex", "prefer birch")
	for i := 1; i <= 8; i++ {
		eventually(t, func() bool { return w.replyCount() == i })
		w.event("whisper", "Bob", "just chatting")
	}
	eventually(t, func() bool { return w.replyCount() == 9 })
	stop()
	before := calls.Load()
	stop2 := start(t, s)
	time.Sleep(40 * time.Millisecond)
	stop2()
	if calls.Load() != before || w.replyCount() != 9 {
		t.Fatal("restart replayed intent/accepted event")
	}
	// A request whose refresh allowance was charged before a crash cannot
	// acquire another model call after restart.
	var cp planner.Checkpoint
	if err := store.Load(context.Background(), &cp); err != nil {
		t.Fatal(err)
	}
	cp.Jobs = append(cp.Jobs, planner.Job{ID: "spent", Lane: "player", Event: "player_request", Status: "planning", Request: "Alex: answer", PlanAttempts: 2})
	if err := store.Commit(context.Background(), cp, nil); err != nil {
		t.Fatal(err)
	}
	stop3 := start(t, s)
	eventually(t, func() bool { return w.replyCount() == 10 })
	stop3()
	if calls.Load() != before {
		t.Fatal("restart refilled planning allowance")
	}
	// Clearing a project on an autonomous result must not swallow its final reply.
	if err := store.Load(context.Background(), &cp); err != nil {
		t.Fatal(err)
	}
	cp.Jobs = []planner.Job{{ID: "finished", Lane: "autonomous", Event: "goal_finished", Status: "queued"}}
	if err := store.Commit(context.Background(), cp, nil); err != nil {
		t.Fatal(err)
	}
	s.Plan = func(context.Context, planner.Input) (planner.Decision, error) {
		clear := ""
		return planner.Decision{Idle: true, Project: &clear, Reply: "The agreed work is complete."}, nil
	}
	stop4 := start(t, s)
	eventually(t, func() bool { return w.replyCount() == 11 })
	stop4()
	var cleared planner.Checkpoint
	if err := store.Load(context.Background(), &cleared); err != nil || cleared.Project != nil {
		t.Fatalf("project not cleared: %+v error=%v", cleared.Project, err)
	}
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
				return store.Load(context.Background(), &c) == nil && c.Recovery != nil && c.Recovery.Closed && c.Recovery.Attempts == 2 && c.Recovery.ClosureReason == "exhausted"
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
				if runs.Load() != 1 || plans.Load() != 2 || c.Recovery == nil || !c.Recovery.Closed || c.Recovery.Attempts != 0 || c.Recovery.ClosureReason != "voluntary_idle" {
					t.Fatalf("idle loop/refill runs=%d plans=%d state=%+v", runs.Load(), plans.Load(), c)
				}
			} else if runs.Load() != 2 || plans.Load() != 3 || c.Recovery != nil || c.AutoPaused {
				t.Fatalf("changed recovery/reset runs=%d plans=%d state=%+v", runs.Load(), plans.Load(), c)
			}
		})
	}
}

func TestRejectedPreserveDoesNotCancelChargedRecovery(t *testing.T) {
	for _, work := range []string{"preserve", "replace"} {
		t.Run(work, func(t *testing.T) {
			w, store := newWorld(), testStore(t)
			w.card.Bot.Health = 2
			cp := planner.Checkpoint{Started: true, Danger: true, AutoPaused: true, Recovery: &planner.RecoveryEpisode{Attempts: 1}, Jobs: []planner.Job{{ID: "recovery", Lane: "autonomous", Status: "queued", PostMortem: true, Context: executor.ExecutionContext{Mode: "danger"}, Goal: target("recovery", 99)}}}
			if err := store.Commit(context.Background(), cp, nil); err != nil {
				t.Fatal(err)
			}
			started := make(chan string, 1)
			var runs, cancelled atomic.Int32
			s := &planner.Scheduler{Memory: store, World: w, PollInterval: 5 * time.Millisecond,
				Check: func(context.Context, goal.Goal) (bool, error) { return false, nil },
				Plan: func(context.Context, planner.Input) (planner.Decision, error) {
					return planner.Decision{Goal: target("new request", 4), Work: work}, nil
				},
				Execute: func(ctx context.Context, _ goal.Goal, _ executor.ExecutionContext) (executor.Outcome, error) {
					runs.Add(1)
					started <- "recovery"
					<-ctx.Done()
					cancelled.Add(1)
					return executor.Outcome{Outcome: executor.Cancelled}, nil
				},
			}
			stop := start(t, s)
			await(t, started, "recovery")
			w.event("whisper", "Alex", "try another task without discarding this one")
			eventually(t, func() bool { return w.replyCount() == 1 })
			if work == "replace" {
				eventually(t, func() bool { return cancelled.Load() == 1 })
			}
			time.Sleep(40 * time.Millisecond)
			var got planner.Checkpoint
			if err := store.Load(context.Background(), &got); err != nil {
				t.Fatal(err)
			}
			w.mu.Lock()
			reply := w.replies[0]
			w.mu.Unlock()
			if !strings.Contains(reply, "attempts 2/2") || got.Recovery == nil || got.Recovery.Attempts != 2 || runs.Load() != 1 {
				t.Fatalf("invalid refusal/charge: reply=%q state=%+v runs=%d", reply, got, runs.Load())
			}
			if work == "preserve" && (cancelled.Load() != 0 || got.Recovery.Closed || len(got.Jobs) != 1 || got.Jobs[0].Status != "running") {
				t.Fatalf("rejected preserve disturbed running recovery: cancelled=%d state=%+v", cancelled.Load(), got)
			}
			stop()
		})
	}
}

func TestRecoveryPreemptionPreservesChargedAllowance(t *testing.T) {
	for _, attempts := range []int{0, 1} {
		t.Run(string(rune('1'+attempts)), func(t *testing.T) {
			w, store := newWorld(), testStore(t)
			cp := planner.Checkpoint{Started: true, AutoPaused: true, Recovery: &planner.RecoveryEpisode{Attempts: attempts}, Jobs: []planner.Job{{ID: "recovery", Lane: "autonomous", Event: "goal_failed", PostMortem: true, Status: "queued", Goal: target("changed approach", 99), Context: executor.ExecutionContext{Mode: "recovery"}}}}
			if err := store.Commit(context.Background(), cp, nil); err != nil {
				t.Fatal(err)
			}
			started, cancelled := make(chan string, 1), make(chan string, 1)
			var runs, plans atomic.Int32
			s := &planner.Scheduler{Memory: store, World: w, PollInterval: 5 * time.Millisecond,
				Check: func(context.Context, goal.Goal) (bool, error) { return false, nil },
				Plan: func(context.Context, planner.Input) (planner.Decision, error) {
					plans.Add(1)
					return planner.Decision{Idle: true, Work: "replace"}, nil
				},
				Execute: func(ctx context.Context, _ goal.Goal, intent executor.ExecutionContext) (executor.Outcome, error) {
					runs.Add(1)
					if intent.Mode != "recovery" || intent.Remaining != 1-attempts {
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
			if attempts == 1 {
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
			}
			w.event("whisper", "Alex", "please stop and reconsider")
			await(t, cancelled, "cleanup")
			eventually(t, func() bool {
				var c planner.Checkpoint
				return store.Load(context.Background(), &c) == nil && c.Recovery != nil && c.Recovery.Closed && c.Recovery.Attempts == attempts+1 && len(c.Jobs) == 0
			})
			stop()
			if runs.Load() != 1 || w.overlap || w.owners != 0 {
				t.Fatalf("preemption refilled/laundered recovery or overlapped cleanup: runs=%d overlap=%v owners=%d", runs.Load(), w.overlap, w.owners)
			}
			stop2 := start(t, s)
			w.mu.Lock()
			w.card.Bot.Health = 2
			w.mu.Unlock()
			w.event("death", "", "another death")
			eventually(t, func() bool {
				var c planner.Checkpoint
				return store.Load(context.Background(), &c) == nil && c.Cursor == 2 && c.Danger
			})
			stop2()
			var got planner.Checkpoint
			if err := store.Load(context.Background(), &got); err != nil {
				t.Fatal(err)
			}
			closure := "explicit_stop"
			if attempts == 1 {
				// New danger closed admission after execution two was charged,
				// before the owner's stop. Preserve that first closure cause.
				closure = "exhausted"
			}
			if plans.Load() != 1 || runs.Load() != 1 || got.Recovery == nil || !got.Recovery.Closed || got.Recovery.Attempts != attempts+1 || got.Recovery.ClosureReason != closure {
				t.Fatalf("restart/notification reopened stopped recovery: plans=%d runs=%d state=%+v", plans.Load(), runs.Load(), got)
			}
		})
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
