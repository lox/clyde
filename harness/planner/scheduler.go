package planner

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lox/clyde/harness/body"
	"github.com/lox/clyde/harness/executor"
	"github.com/lox/clyde/harness/goal"
	"github.com/lox/clyde/harness/judge"
	"github.com/lox/clyde/harness/memory"
)

// World is the subset of the body used by the scheduler. Chat actions must be
// non-replacing; only the executor owns movement and other world actions.
type World interface {
	State(context.Context) (body.Card, error)
	Events(context.Context, int) (body.EventPage, error)
	Stop(context.Context) error
	Action(context.Context, any) (*body.ActionStatus, error)
}

// Job is persisted before planning/execution. Interrupted autonomous goals keep
// their typed predicate but get a fresh executor on resumption.
type Job struct {
	ID           string                    `json:"id"`
	Lane         string                    `json:"lane"`
	Event        string                    `json:"event"`
	Request      string                    `json:"request,omitempty"`
	Goal         *goal.Goal                `json:"goal,omitempty"`
	Status       string                    `json:"status"` // queued, planning, running
	Accepted     time.Time                 `json:"accepted"`
	ReplySent    bool                      `json:"reply_sent"` // persisted send intent: see runbook crash window
	PostMortem   bool                      `json:"post_mortem,omitempty"`
	Context      executor.ExecutionContext `json:"execution_context,omitempty"`
	PlanAttempts int                       `json:"plan_attempts,omitempty"`
	PlanUntil    time.Time                 `json:"plan_until,omitempty"`
	Superseded   bool                      `json:"superseded,omitempty"`
}

// RecoveryEpisode survives idle, exhaustion and restarts. Only a successful
// execution followed by a safe observation resets it; notifications never do.
type RecoveryEpisode struct {
	Attempts      int    `json:"attempts"`
	Closed        bool   `json:"closed"`
	ClosureReason string `json:"closure_reason,omitempty"`
}

func (r *RecoveryEpisode) close(reason string) {
	if !r.Closed {
		r.Closed, r.ClosureReason = true, reason
	}
}

const maxRecovery = 2

const maxDialogue = 12

// Turn is addressed dialogue, in chronological order. Clyde's entries are send
// intents, not acknowledgements of delivery; request IDs link both sides.
type Turn struct {
	RequestID   string `json:"request_id"`
	Speaker     string `json:"speaker"`
	Text        string `json:"text"`
	ReplyIntent bool   `json:"reply_intent,omitempty"`
}

// Project is one model-written intent, not observed progress or a workflow.
// The originating request is retained separately for provenance. Verification
// remains in goal-result episodes and live predicates, never this summary.
type Project struct {
	Summary   string `json:"summary"`
	Request   string `json:"source_request"`
	RequestID string `json:"source_request_id"`
}

// Checkpoint is the durable queue and body-event high-water mark. Event IDs,
// recorded atomically with this checkpoint, disambiguate body sequence resets.
type Checkpoint struct {
	Jobs           []Job            `json:"jobs"`
	Cursor         int              `json:"cursor"`
	CursorTS       int64            `json:"cursor_ts"`
	Started        bool             `json:"started"`
	Danger         bool             `json:"danger"`
	AutoPaused     bool             `json:"auto_paused"`
	PauseReason    string           `json:"pause_reason,omitempty"`
	NextID         uint64           `json:"next_id"`
	AutonomousRuns int              `json:"autonomous_runs"`
	Recovery       *RecoveryEpisode `json:"recovery,omitempty"`
	Dialogue       []Turn           `json:"dialogue,omitempty"`
	Project        *Project         `json:"project,omitempty"`
}

// Scheduler owns one serve session. All state transitions happen in Run's
// goroutine. One read-only planner may answer beside the exclusive executor;
// a replacement executor starts only after the old worker's cleanup returns.
type Scheduler struct {
	Memory        *memory.Store
	World         World
	Plan          func(context.Context, Input) (Decision, error)
	Execute       func(context.Context, goal.Goal, executor.ExecutionContext) (executor.Outcome, error)
	Check         func(context.Context, goal.Goal) (bool, error)
	SaveCandidate func(context.Context, Candidate) error
	PollInterval  time.Duration // body I/O only, never periodic model invocation
	PlanningTime  time.Duration // per-request deadline, shared by player refreshes
	MaxAutonomous int           // default 10 completed autonomous runs per durable session
	Log           io.Writer
}

type workResult struct {
	id       string
	phase    string
	decision Decision
	out      executor.Outcome
	err      error
	revision uint64
}

// Run reconciles outstanding work then consumes events until graceful shutdown.
// A runner must not return before its body stop cleanup; a failed independent
// stop is fatal rather than granting another executor uncertain body ownership.
func (s *Scheduler) Run(ctx context.Context) error {
	if s.Memory == nil || s.World == nil || s.Plan == nil || s.Execute == nil || s.Check == nil {
		return errors.New("scheduler: memory, world, planner, executor and checker required")
	}
	if s.PollInterval <= 0 {
		s.PollInterval = 250 * time.Millisecond
	}
	if s.MaxAutonomous <= 0 {
		s.MaxAutonomous = 10
	}
	if s.PlanningTime <= 0 {
		s.PlanningTime = 30 * time.Second
	}
	if s.Log == nil {
		s.Log = io.Discard
	}
	var state Checkpoint
	if err := s.Memory.Load(ctx, &state); err != nil {
		return err
	}
	commit := func(eps ...memory.Episode) error { return s.Memory.Commit(ctx, state, eps) }
	// Halt stale actions first. A crashed executor cannot keep acting while its
	// predicate is reconciled or while a new request gets planned.
	if err := s.stop(ctx); err != nil {
		return err
	}
	for i := 0; i < len(state.Jobs); {
		j := &state.Jobs[i]
		if j.Superseded {
			// Cancellation intent was committed with the replacing decision.
			// Startup already stopped stale body work; never replay this goal.
			state.Jobs = append(state.Jobs[:i], state.Jobs[i+1:]...)
			continue
		}
		if j.Status == "running" && j.Goal != nil {
			cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			held, err := s.Check(cctx, *j.Goal)
			cancel()
			if err != nil {
				return fmt.Errorf("reconcile %s: %w", j.ID, err)
			}
			if held {
				e := memory.Episode{Kind: "reconciled_succeeded", Detail: j.ID + ": " + j.Goal.Description}
				if j.Lane == "autonomous" {
					state.AutonomousRuns++
				}
				card, err := s.World.State(ctx)
				if err != nil {
					return err
				}
				if len(judge.Dangers(card, judge.DefaultConfig().Prechecks)) == 0 {
					state.Recovery, state.AutoPaused, state.PauseReason = nil, false, ""
				}
				state.Jobs = append(state.Jobs[:i], state.Jobs[i+1:]...)
				if state.Recovery == nil {
					s.enqueue(&state, "goal_finished", false)
				} else {
					s.recover(&state, "goal_finished")
				}
				if err := commit(e); err != nil {
					return err
				}
				continue
			}
			if j.Context.Mode != "" {
				// The attempt was charged before dispatch. A crash is not a free
				// replay of that attempt, even when the predicate is unsatisfied.
				state.Jobs = append(state.Jobs[:i], state.Jobs[i+1:]...)
				s.recover(&state, "restart_interrupted")
				continue
			}
		}
		j.Status = "queued"
		i++
	}
	if !state.Started {
		state.Started = true
		s.enqueue(&state, "idle", false)
	}
	if err := commit(memory.Episode{Kind: "start", Detail: "unfinished goals checked against live predicates; unsatisfied work requeued"}); err != nil {
		return err
	}
	// A single result queue orders planner decisions against executor results.
	// Only control transitions invalidate planning, never ordinary movement.
	results := make(chan workResult, 2)
	var cancelPlan, cancelRun context.CancelFunc
	var planID, runID, preempt string
	var revision uint64
	// Always join both workers; neither a model call nor a body owner outlives Run.
	defer func() {
		pending := 0
		for _, cancel := range []context.CancelFunc{cancelPlan, cancelRun} {
			if cancel != nil {
				cancel()
				pending++
			}
		}
		for range pending {
			<-results
		}
	}()
	fail := func(err error) error {
		if ctx.Err() != nil {
			return s.shutdown(ctx, &state, &cancelPlan, &cancelRun, results)
		}
		return err
	}
	ticker := time.NewTicker(s.PollInterval)
	defer ticker.Stop()
	var card body.Card
	for {
		// Polling also runs immediately at startup and after completion, so queued
		// requests are noticed before an autonomous worker can start.
		pollCtx, pollCancel := context.WithTimeout(ctx, 3*time.Second)
		wasDanger := state.Danger
		newPlayer, death, err := s.poll(pollCtx, &state, &card)
		pollCancel()
		if err != nil {
			return fail(err)
		}
		if death || wasDanger != state.Danger {
			revision++
			if cancelPlan != nil {
				cancelPlan()
			}
		}
		if planID != "" && newPlayer && findJob(&state, planID).Lane == "autonomous" {
			revision++
			cancelPlan()
		}
		if runID != "" {
			j := findJob(&state, runID)
			dangerPreempt := state.Danger && j.Context.Mode == ""
			if preempt == "" && (death || dangerPreempt) {
				preempt = "danger"
				if death {
					preempt = "death"
				}
				cancelRun()
			}
		}
		if planID == "" {
			idx := -1
			if runID == "" {
				idx = nextJob(state)
			} else if preempt == "" {
				// Conversation may be planned beside work, but never another
				// executor or autonomous continuation based on unfinished work.
				for i, j := range state.Jobs {
					if j.Lane == "player" && j.Goal == nil {
						idx = i
						break
					}
				}
			}
			if idx >= 0 {
				j := &state.Jobs[idx]
				if j.Goal != nil && j.Lane == "autonomous" && state.AutonomousRuns >= s.MaxAutonomous {
					state.AutoPaused = true
					state.PauseReason = "autonomous_goal_limit"
					removeJob(&state, j.ID)
					if err := commit(memory.Episode{Kind: "budget", Detail: "autonomous session goal budget reached"}); err != nil {
						return fail(err)
					}
					continue
				}
				phase := "planning"
				if j.Goal != nil {
					phase = "running"
				}
				if phase == "planning" && j.Lane == "player" {
					if j.PlanAttempts >= 2 || (!j.PlanUntil.IsZero() && time.Now().After(j.PlanUntil)) {
						if err := s.finish(ctx, &state, workResult{id: j.ID, phase: phase, err: errors.New("request planning allowance exhausted")}, ""); err != nil {
							return fail(err)
						}
						continue
					}
					if j.PlanUntil.IsZero() {
						j.PlanUntil = time.Now().Add(s.PlanningTime)
					}
					j.PlanAttempts++ // durable: restart/stale results do not mint calls
				}
				if j.PostMortem || (j.Lane == "player" && state.Danger) {
					if state.Recovery == nil {
						state.Recovery = &RecoveryEpisode{}
					}
					j.Context = executor.ExecutionContext{Mode: "recovery", Cause: j.Event}
					if state.Danger || j.Event == "danger" {
						j.Context.Mode = "danger"
						j.Context.Danger = judge.Dangers(card, judge.DefaultConfig().Prechecks)
					}
					j.Context.Remaining = maxRecovery - state.Recovery.Attempts
				}
				if phase == "running" && j.Context.Mode != "" {
					if state.Recovery == nil {
						state.Recovery = &RecoveryEpisode{}
					}
					if state.Recovery.Closed || state.Recovery.Attempts >= maxRecovery {
						state.Recovery.close("exhausted")
						removeJob(&state, j.ID)
						if err := commit(memory.Episode{Kind: "recovery_exhausted", Detail: "recovery allowance unavailable; idle until successful safe player work"}); err != nil {
							return fail(err)
						}
						continue
					}
					state.Recovery.Attempts++
					j.Context.Remaining = maxRecovery - state.Recovery.Attempts
				}
				j.Status = phase
				if err := commit(memory.Episode{Kind: phase, Detail: j.ID + ": " + j.Event + " " + j.Request}); err != nil {
					return fail(err)
				}
				job := *j
				in, err := s.input(ctx, job, card, state)
				if err != nil {
					return fail(err)
				}
				job.Context = in.Context
				workCtx, cancel := context.WithCancel(ctx)
				if phase == "planning" {
					cancel()
					deadline := time.Now().Add(s.PlanningTime)
					if job.Lane == "player" && job.PlanUntil.Before(deadline) {
						deadline = job.PlanUntil
					}
					workCtx, cancel = context.WithDeadline(ctx, deadline)
					planID, cancelPlan = job.ID, cancel
				} else {
					runID, cancelRun = job.ID, cancel
				}
				startedRevision := revision
				go func() {
					r := workResult{id: job.ID, phase: phase, revision: startedRevision}
					if phase == "planning" {
						r.decision, r.err = s.Plan(workCtx, in)
					} else {
						r.out, r.err = s.Execute(workCtx, *job.Goal, job.Context)
					}
					results <- r
				}()
			}
		}
		select {
		case <-ctx.Done():
			return s.shutdown(ctx, &state, &cancelPlan, &cancelRun, results)
		case <-ticker.C:
			continue
		case r := <-results:
			if r.phase == "planning" {
				cancelPlan()
				cancelPlan, planID = nil, ""
				stale := ""
				if r.revision != revision || preempt != "" {
					stale = "control_changed"
				}
				// Admit the goal before deciding whether it needs the executor.
				// A rejected preserve must not interrupt an already charged attempt.
				j := findJob(&state, r.id)
				if stale == "" && r.err == nil && r.decision.Goal != nil && j.Context.Mode != "" && state.Recovery != nil && (state.Recovery.Closed || state.Recovery.Attempts >= maxRecovery) {
					r.decision.Goal = nil
					r.decision.Idle = true
					r.decision.Reply = fmt.Sprintf("Recovery admission unavailable (attempts %d/%d, closed=%t, closure=%s). A safe ordinary player request does not require recovery admission.", state.Recovery.Attempts, maxRecovery, state.Recovery.Closed, recoveryClosure(state.Recovery))
				}
				if stale == "" && r.err == nil && (r.decision.Work == "replace" || r.decision.Goal != nil) && runID != "" {
					preempt = "player"
					if r.decision.Work == "replace" {
						preempt = "replace"
					}
					// Do not keep obsolete work acting while sending its reply.
					// The replacement still cannot start until cleanup is joined.
					cancelRun()
				}
				finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				err := s.finish(finishCtx, &state, r, stale)
				cancel()
				if err != nil {
					return err
				}
				continue
			}
			cancelRun()
			cancelRun = nil
			revision++
			if cancelPlan != nil {
				cancelPlan()
			}
			// A second body stop confirms ownership even if a runner's cleanup
			// logged and swallowed a stop failure. Never overlap uncertain owners.
			if r.phase == "running" {
				if err := s.stop(ctx); err != nil {
					return err
				}
				// Success is not proof of safety (a recovery predicate may be a
				// small inventory milestone). Reset only after a fresh observation.
				if r.out.Outcome == executor.Succeeded && r.err == nil {
					cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
					fresh, err := s.World.State(cctx)
					cancel()
					if err != nil {
						return err
					}
					state.Danger = len(judge.Dangers(fresh, judge.DefaultConfig().Prechecks)) > 0
				}
			}
			// Once a result is received, finish its durable transition even if
			// shutdown arrives concurrently. Otherwise a completed reply/result
			// can be lost merely because its SQL context was cancelled.
			finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			err := s.finish(finishCtx, &state, r, preempt)
			finishCancel()
			if err != nil {
				return err
			}
			runID, preempt = "", ""
		}
	}
}

func (s *Scheduler) stop(ctx context.Context) error {
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := s.World.Stop(cctx); err != nil {
		return fmt.Errorf("scheduler: body ownership not released: %w", err)
	}
	return nil
}

func (s *Scheduler) shutdown(ctx context.Context, state *Checkpoint, cancelPlan, cancelRun *context.CancelFunc, results <-chan workResult) error {
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	// Poll may have been cancelled before its transaction committed. Restore
	// the durable checkpoint rather than persisting partially accepted events.
	var durable Checkpoint
	if err := s.Memory.Load(cctx, &durable); err != nil {
		return err
	}
	*state = durable
	eps := []memory.Episode{{Kind: "shutdown", Detail: "queue retained; in-flight goals require live reconciliation"}}
	pending := 0
	for _, worker := range []*context.CancelFunc{cancelPlan, cancelRun} {
		if *worker != nil {
			(*worker)()
			*worker = nil
			pending++
		}
	}
	for range pending {
		r := <-results
		eps[0].Tokens += r.decision.Tokens + r.out.Tokens.Total
		eps[0].CostUSD += r.decision.CostUSD + r.out.CostUSD
		j := findJob(state, r.id)
		// Leave execution marked running for live reconciliation after restart.
		if r.phase == "planning" {
			j.Status = "queued"
		}
	}
	err := s.stop(ctx)
	return errors.Join(err, s.Memory.Commit(cctx, *state, eps))
}

func (s *Scheduler) enqueue(st *Checkpoint, event string, postMortem bool) {
	// Coalesce autonomous events; never turn repeated idle cards into planning.
	for _, j := range st.Jobs {
		if j.Lane == "autonomous" && j.Event == event && j.Goal == nil {
			return
		}
	}
	st.NextID++
	st.Jobs = append(st.Jobs, Job{ID: fmt.Sprintf("auto-%d", st.NextID), Lane: "autonomous", Event: event, Status: "queued", Accepted: time.Now().UTC(), PostMortem: postMortem})
}

func (s *Scheduler) recover(st *Checkpoint, event string) {
	if !st.AutoPaused {
		st.PauseReason = event
	}
	st.AutoPaused = true
	if st.Recovery == nil {
		st.Recovery = &RecoveryEpisode{}
	}
	if st.Recovery.Attempts >= maxRecovery {
		st.Recovery.close("exhausted")
	}
	if st.Recovery.Closed {
		return
	}
	for _, j := range st.Jobs {
		if j.PostMortem {
			return
		}
	}
	s.enqueue(st, event, true)
}

func nextJob(st Checkpoint) int {
	for i, j := range st.Jobs {
		if j.Lane == "player" && (!st.Danger || j.Goal == nil || j.Context.Mode != "") {
			return i
		}
	}
	for i, j := range st.Jobs {
		if j.PostMortem {
			return i
		}
	}
	for i := range st.Jobs {
		if !st.Danger && !st.AutoPaused {
			return i
		}
	}
	return -1
}

func findJob(st *Checkpoint, id string) *Job {
	for i := range st.Jobs {
		if st.Jobs[i].ID == id {
			return &st.Jobs[i]
		}
	}
	panic("scheduler: active job missing")
}

func removeJob(st *Checkpoint, id string) {
	for i := range st.Jobs {
		if st.Jobs[i].ID == id {
			st.Jobs = append(st.Jobs[:i], st.Jobs[i+1:]...)
			return
		}
	}
}

func (s *Scheduler) input(ctx context.Context, j Job, card body.Card, st Checkpoint) (Input, error) {
	episodes, err := s.Memory.Recent(ctx, 16)
	if err != nil {
		return Input{}, err
	}
	places, err := s.Memory.Places(ctx)
	if err != nil {
		return Input{}, err
	}
	semantic, err := s.Memory.Semantic()
	if err != nil {
		return Input{}, err
	}
	b, err := json.Marshal(card)
	var work []Job
	for _, job := range st.Jobs {
		if job.Goal != nil {
			work = append(work, job)
		}
	}
	j.Context.Control = s.control(j, card, st)
	return Input{Context: j.Context, Event: j.Event, Request: j.Request, Dialogue: append([]Turn(nil), st.Dialogue...), Project: st.Project, Work: work, State: string(b), Episodes: episodes, Places: places, Semantic: semantic}, err
}

func recoveryClosure(r *RecoveryEpisode) string {
	if r == nil || !r.Closed {
		return "none"
	}
	if r.ClosureReason == "" {
		return "unknown" // Do not retroactively label a legacy closed-zero episode.
	}
	return r.ClosureReason
}

func (s *Scheduler) control(j Job, card body.Card, st Checkpoint) *executor.ControlFacts {
	f := &executor.ControlFacts{RequestID: j.ID, Admission: "ordinary", Limit: maxRecovery,
		AutoPaused: st.AutoPaused, PauseReason: st.PauseReason, ClosureReason: recoveryClosure(st.Recovery)}
	if st.AutoPaused && f.PauseReason == "" {
		f.PauseReason = "unknown"
	}
	if st.Recovery != nil {
		f.Attempts, f.Closed = st.Recovery.Attempts, st.Recovery.Closed
	}
	for _, job := range st.Jobs {
		if job.Status == "running" {
			f.ExecutorOwner = job.ID
		}
	}
	if card.Action != nil {
		f.ActionOwner = card.Action.Token
	}
	f.Eligible = !st.Danger && (j.Lane == "player" || (!st.AutoPaused && st.AutonomousRuns < s.MaxAutonomous))
	if j.Context.Mode != "" {
		f.Admission = "recovery"
		f.Eligible = !f.Closed && f.Attempts < maxRecovery
		if j.Status == "running" {
			f.Admission, f.Eligible = "recovery_charged", true
		}
	}
	return f
}

func dialogue(st *Checkpoint, turn Turn) {
	st.Dialogue = append(st.Dialogue, turn)
	if len(st.Dialogue) > maxDialogue {
		st.Dialogue = append([]Turn(nil), st.Dialogue[len(st.Dialogue)-maxDialogue:]...)
	}
}

func (s *Scheduler) poll(ctx context.Context, st *Checkpoint, card *body.Card) (newPlayer, death bool, err error) {
	*card, err = s.World.State(ctx)
	if err != nil {
		return
	}
	// Read the bounded ring, using durable content identities as the cursor.
	// A sequence-only since=N cannot detect a new body whose counter has already
	// passed N; replaying the ring detects that reset without losing its prefix.
	var page body.EventPage
	page, err = s.World.Events(ctx, 0)
	if err != nil {
		return
	}
	var eps []memory.Episode
	var keys []string
	if page.LatestSeq < st.Cursor {
		eps = append(eps, memory.Episode{Kind: "event_reset", Detail: "body event sequence reset"})
	}
	if len(page.Events) > 0 && st.CursorTS > 0 && page.Events[0].TS > st.CursorTS && page.Events[0].Seq > st.Cursor+1 {
		eps = append(eps, memory.Episode{Kind: "event_gap", Detail: fmt.Sprintf("events before seq %d unavailable; cannot reconstruct missing chat", page.Events[0].Seq)})
	}
	for _, e := range page.Events {
		b, _ := json.Marshal(e)
		key := fmt.Sprintf("%x", sha256.Sum256(b))
		var seen bool
		seen, err = s.Memory.Seen(ctx, key)
		if err != nil {
			return
		}
		if seen {
			continue
		}
		keys = append(keys, key)
		switch e.Type {
		case "chat", "whisper":
			username, _ := e.Data["username"].(string)
			message, _ := e.Data["message"].(string)
			if username == "" || strings.EqualFold(username, card.Bot.Name) {
				continue
			}
			eps = append(eps, memory.Episode{Kind: "chat", Detail: clip(e.Text, 2000)})
			// Whisper is addressed by definition. Public chat must start with the
			// bot's name (optional ':' or ','); ordinary conversation is not work.
			if e.Type == "chat" {
				fields := strings.Fields(message)
				if len(fields) < 2 || !strings.EqualFold(strings.TrimRight(fields[0], ":,"), card.Bot.Name) {
					continue
				}
				message = strings.TrimSpace(message[len(fields[0]):])
			}
			if strings.TrimSpace(message) == "" {
				continue
			}
			if len(st.Jobs) >= 64 {
				eps = append(eps, memory.Episode{Kind: "request_rejected", Detail: "queue full: " + clip(message, 200)})
				continue
			}
			st.Jobs = append(st.Jobs, Job{ID: key, Lane: "player", Event: "player_request", Request: username + ": " + clip(message, 2000), Status: "queued", Accepted: time.Now().UTC()})
			dialogue(st, Turn{RequestID: key, Speaker: username, Text: clip(message, 2000)})
			newPlayer = true
			eps = append(eps, memory.Episode{Kind: "request_accepted", Detail: key + ": " + clip(message, 2000)})
		case "death":
			death = true
			s.recover(st, "death")
			eps = append(eps, memory.Episode{Kind: "death", Detail: e.Text})
		}
	}
	st.Cursor = page.LatestSeq
	if len(page.Events) > 0 {
		st.CursorTS = page.Events[len(page.Events)-1].TS
	}
	danger := len(judge.Dangers(*card, judge.DefaultConfig().Prechecks)) > 0
	if danger && !st.Danger {
		s.recover(st, "judge_interrupt")
		eps = append(eps, memory.Episode{Kind: "danger", Detail: "ordinary work preempted; bounded model response eligible"})
	} else if !danger && st.Danger {
		eps = append(eps, memory.Episode{Kind: "danger_cleared", Detail: "live prechecks safe; allowance unchanged until successful execution"})
	}
	st.Danger = danger
	if err = s.Memory.Commit(ctx, *st, eps, keys...); err != nil {
		return
	}
	// Home is explicitly just the first observed anchor, not an inferred bed.
	// Resources/POIs are the nearest visible notable block of each type.
	at := time.UnixMilli(int64(card.Ts)).UTC()
	p := card.Bot.Position
	err = s.Memory.Observe(ctx, memory.Place{Name: "home:first-observed", Kind: "home", Dimension: card.Bot.Dimension, X: p.X, Y: p.Y, Z: p.Z, At: at})
	if err != nil {
		return
	}
	for _, p := range card.Entities.Players {
		err = s.Memory.Observe(ctx, memory.Place{Name: "player:" + p.Name, Kind: "player", Dimension: card.Bot.Dimension, X: p.Position.X, Y: p.Position.Y, Z: p.Position.Z, At: at})
		if err != nil {
			return
		}
	}
	for _, b := range card.Blocks {
		kind := "poi"
		if strings.Contains(b.Name, "log") || strings.Contains(b.Name, "ore") {
			kind = "resource"
		}
		err = s.Memory.Observe(ctx, memory.Place{Name: kind + ":" + card.Bot.Dimension + ":" + b.Name, Kind: kind, Dimension: card.Bot.Dimension, X: b.NearestPos.X, Y: b.NearestPos.Y, Z: b.NearestPos.Z, At: at})
		if err != nil {
			return
		}
	}
	return
}

func (s *Scheduler) finish(ctx context.Context, st *Checkpoint, r workResult, preempt string) error {
	j := findJob(st, r.id)
	var eps []memory.Episode
	if r.phase == "planning" {
		d := r.decision
		eps = append(eps, memory.Episode{Kind: "planning_result", Detail: fmt.Sprintf("%s event=%s error=%v", j.ID, j.Event, r.err), Tokens: d.Tokens, CostUSD: d.CostUSD})
		if preempt != "" {
			j.Status = "queued"
			return s.Memory.Commit(ctx, *st, eps)
		}
		if r.err != nil {
			d = Decision{Idle: true, Reply: "I couldn't make a valid plan within my budget; please clarify or try again."}
		}
		for _, n := range d.Notes {
			if err := s.Memory.Curate(ctx, n.Name, n.Markdown, n.Episodes); err != nil {
				eps = append(eps, memory.Episode{Kind: "note_rejected", Detail: err.Error()})
			}
		}
		if d.Candidate != nil {
			var err error
			if s.SaveCandidate == nil {
				err = errors.New("candidate store not configured")
			} else {
				err = s.SaveCandidate(ctx, *d.Candidate)
			}
			kind := "candidate_saved"
			if err != nil {
				kind = "candidate_rejected"
			}
			eps = append(eps, memory.Episode{Kind: kind, Detail: fmt.Sprintf("%s: %v (not admitted)", d.Candidate.Name, err)})
		}
		j.Goal = d.Goal
		j.Status = "queued"
		hadProject := st.Project != nil
		if d.Project != nil {
			if *d.Project == "" {
				st.Project = nil
			} else {
				source := j.Request
				sourceID := j.ID
				if source == "" && st.Project != nil {
					source = st.Project.Request
					sourceID = st.Project.RequestID
				}
				st.Project = &Project{Summary: *d.Project, Request: source, RequestID: sourceID}
			}
		}
		sendReply := d.Reply != "" && (j.Lane == "player" || hadProject || st.Project != nil) && !j.ReplySent
		if sendReply {
			j.ReplySent = true // persist intent BEFORE network I/O: at-most-once attempt
			dialogue(st, Turn{RequestID: j.ID, Speaker: "Clyde", Text: d.Reply, ReplyIntent: true})
			eps = append(eps, memory.Episode{Kind: "reply_intent", Detail: j.ID + ": " + d.Reply})
		}
		job := *j
		if d.Work == "replace" {
			kept := st.Jobs[:0]
			for _, queued := range st.Jobs {
				// A running owner stays until its joined result is processed.
				// Pending player turns and recovery notifications are not erased.
				if queued.ID != job.ID && queued.Status == "running" {
					queued.Superseded = true
				}
				if queued.ID == job.ID || queued.Goal == nil || queued.Status == "running" {
					kept = append(kept, queued)
				}
			}
			st.Jobs = kept
		}
		if d.Goal == nil {
			if st.Recovery != nil && (d.Work == "replace" || (job.Context.Mode != "" && job.PostMortem)) {
				reason := "voluntary_idle"
				if d.Work == "replace" {
					reason = "explicit_stop"
				}
				st.Recovery.close(reason)
			}
			// Persist terminal idle/refusal along with send intent. A crash after
			// chat must not make the answered request eligible for replanning.
			eps = append(eps, memory.Episode{Kind: "idle", Detail: job.ID + ": " + d.Reply})
			removeJob(st, r.id)
		} else {
			// The new player goal takes priority over preserved/suspended work.
			removeJob(st, r.id)
			st.Jobs = append([]Job{job}, st.Jobs...)
		}
		decision, _ := json.Marshal(d)
		eps = append(eps, memory.Episode{Kind: "decision_applied", Detail: job.ID + ": " + string(decision)})
		if sendReply {
			if err := s.Memory.Commit(ctx, *st, eps); err != nil {
				return err
			}
			eps = nil
			_, err := s.World.Action(ctx, map[string]any{"type": "chat", "message": d.Reply})
			eps = append(eps, memory.Episode{Kind: "reply_attempt", Detail: fmt.Sprintf("%s latency_ms=%d reply=%s error=%v", job.ID, time.Since(job.Accepted).Milliseconds(), d.Reply, err)})
		}
	} else {
		if j.Superseded {
			preempt = "replace"
		}
		status := r.out.Outcome
		if r.err != nil {
			status = executor.Failed
		}
		eps = append(eps, memory.Episode{Kind: "goal_" + status, Detail: fmt.Sprintf("%s %s: %s %s error=%v", j.ID, j.Goal.Description, r.out.Detail, r.out.Until, r.err), Tokens: r.out.Tokens.Total, CostUSD: r.out.CostUSD})
		if preempt != "" && status != executor.Succeeded && preempt != "death" && preempt != "replace" {
			j.Status = "queued"
			eps = append(eps, memory.Episode{Kind: "requeued", Detail: j.ID + ": " + preempt})
		} else {
			lane := j.Lane
			removeJob(st, r.id)
			if status == executor.Succeeded {
				if lane == "autonomous" {
					st.AutonomousRuns++
				}
				if !st.Danger {
					st.Recovery, st.AutoPaused, st.PauseReason = nil, false, ""
					eps = append(eps, memory.Episode{Kind: "recovery_reset", Detail: "successful execution and fresh safe observation"})
					s.enqueue(st, "goal_finished", false)
				} else {
					s.recover(st, "goal_finished")
				}
			} else if preempt != "replace" {
				event := "goal_failed"
				if r.out.Interrupt != nil {
					event = "judge_interrupt"
					if r.out.Interrupt.Reason == "danger" {
						event = "danger"
					}
				}
				s.recover(st, event)
			}
		}
	}
	for _, e := range eps {
		fmt.Fprintf(s.Log, "%s: %s tokens=%d cost_usd=%.6f\n", e.Kind, e.Detail, e.Tokens, e.CostUSD)
	}
	return s.Memory.Commit(ctx, *st, eps)
}
