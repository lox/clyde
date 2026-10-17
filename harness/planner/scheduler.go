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

// World is the subset of the body used by the scheduler. Chat is sent only
// between executor runs because even a chat action replaces the current action.
type World interface {
	State(context.Context) (body.Card, error)
	Events(context.Context, int) (body.EventPage, error)
	Stop(context.Context) error
	Action(context.Context, any) (*body.ActionStatus, error)
}

// Job is persisted before planning/execution. Interrupted autonomous goals keep
// their typed predicate but get a fresh executor on resumption.
type Job struct {
	ID         string     `json:"id"`
	Lane       string     `json:"lane"`
	Event      string     `json:"event"`
	Request    string     `json:"request,omitempty"`
	Goal       *goal.Goal `json:"goal,omitempty"`
	Status     string     `json:"status"` // queued, planning, running
	Accepted   time.Time  `json:"accepted"`
	ReplySent  bool       `json:"reply_sent"` // persisted send intent: see runbook crash window
	PostMortem bool       `json:"post_mortem,omitempty"`
}

// Checkpoint is the durable queue and body-event high-water mark. Event IDs,
// recorded atomically with this checkpoint, disambiguate body sequence resets.
type Checkpoint struct {
	Jobs           []Job  `json:"jobs"`
	Cursor         int    `json:"cursor"`
	CursorTS       int64  `json:"cursor_ts"`
	Started        bool   `json:"started"`
	Danger         bool   `json:"danger"`
	AutoPaused     bool   `json:"auto_paused"`
	NextID         uint64 `json:"next_id"`
	AutonomousRuns int    `json:"autonomous_runs"`
}

// Scheduler owns one serve session. All state transitions happen in Run's
// goroutine; the one worker sends a result only after executor cleanup returns.
type Scheduler struct {
	Memory        *memory.Store
	World         World
	Plan          func(context.Context, Input) (Decision, error)
	Execute       func(context.Context, goal.Goal) (executor.Outcome, error)
	Check         func(context.Context, goal.Goal) (bool, error)
	SaveCandidate func(context.Context, Candidate) error
	PollInterval  time.Duration // body I/O only, never periodic model invocation
	MaxAutonomous int           // default 10 completed autonomous runs per durable session
	Log           io.Writer
}

type workResult struct {
	id       string
	phase    string
	decision Decision
	out      executor.Outcome
	err      error
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
				state.Jobs = append(state.Jobs[:i], state.Jobs[i+1:]...)
				s.enqueue(&state, "goal_finished", false)
				if err := commit(e); err != nil {
					return err
				}
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
	results := make(chan workResult, 1)
	var cancelWork context.CancelFunc
	var activeID, preempt string
	// Always join the worker on exit; no goroutine or body owner outlives Run.
	defer func() {
		if cancelWork != nil {
			cancelWork()
			<-results
		}
	}()
	ticker := time.NewTicker(s.PollInterval)
	defer ticker.Stop()
	var card body.Card
	for {
		// Polling also runs immediately at startup and after completion, so queued
		// requests are noticed before an autonomous worker can start.
		pollCtx, pollCancel := context.WithTimeout(ctx, 3*time.Second)
		newPlayer, death, err := s.poll(pollCtx, &state, &card)
		pollCancel()
		if err != nil {
			if ctx.Err() != nil {
				return s.shutdown(ctx, &state, activeID, &cancelWork, results)
			}
			return err
		}
		if activeID != "" {
			j := findJob(&state, activeID)
			if preempt == "" && (death || (state.Danger && !j.PostMortem) || (newPlayer && j.Lane == "autonomous")) {
				preempt = "player"
				if death {
					preempt = "death"
				} else if state.Danger {
					preempt = "danger"
				}
				cancelWork()
			}
		}
		if activeID == "" {
			idx := nextJob(state)
			if idx >= 0 {
				j := &state.Jobs[idx]
				if j.Goal != nil && j.Lane == "autonomous" && state.AutonomousRuns >= s.MaxAutonomous {
					state.AutoPaused = true
					if err := commit(memory.Episode{Kind: "budget", Detail: "autonomous session goal budget reached"}); err != nil {
						return err
					}
					continue
				}
				phase := "planning"
				if j.Goal != nil {
					phase = "running"
				}
				j.Status = phase
				if err := commit(memory.Episode{Kind: phase, Detail: j.ID + ": " + j.Event + " " + j.Request}); err != nil {
					return err
				}
				activeID = j.ID
				workCtx, cancel := context.WithCancel(ctx)
				cancelWork = cancel
				job := *j
				in, err := s.input(ctx, job, card)
				if err != nil {
					cancelWork = nil
					cancel()
					return err
				}
				go func() {
					r := workResult{id: job.ID, phase: phase}
					if phase == "planning" {
						r.decision, r.err = s.Plan(workCtx, in)
					} else {
						r.out, r.err = s.Execute(workCtx, *job.Goal)
					}
					results <- r
				}()
			}
		}
		select {
		case <-ctx.Done():
			return s.shutdown(ctx, &state, activeID, &cancelWork, results)
		case <-ticker.C:
			continue
		case r := <-results:
			cancelWork()
			cancelWork = nil
			// A second body stop confirms ownership even if a runner's cleanup
			// logged and swallowed a stop failure. Never overlap uncertain owners.
			if r.phase == "running" {
				if err := s.stop(ctx); err != nil {
					return err
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
			activeID, preempt = "", ""
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

func (s *Scheduler) shutdown(ctx context.Context, state *Checkpoint, id string, cancelWork *context.CancelFunc, results <-chan workResult) error {
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	// Poll may have been cancelled before its transaction committed. Restore
	// the durable checkpoint rather than persisting partially accepted events.
	if err := s.Memory.Load(cctx, state); err != nil {
		return err
	}
	eps := []memory.Episode{{Kind: "shutdown", Detail: "queue retained; in-flight goals require live reconciliation"}}
	if *cancelWork != nil {
		(*cancelWork)()
		r := <-results
		*cancelWork = nil
		eps[0].Tokens = r.decision.Tokens + r.out.Tokens.Total
		eps[0].CostUSD = r.decision.CostUSD + r.out.CostUSD
		j := findJob(state, id)
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

func nextJob(st Checkpoint) int {
	if st.Danger {
		for i, j := range st.Jobs {
			if j.PostMortem {
				return i
			}
		}
		return -1
	}
	for i, j := range st.Jobs {
		if j.Lane == "player" {
			return i
		}
	}
	for i, j := range st.Jobs {
		if j.PostMortem {
			return i
		}
	}
	for i, j := range st.Jobs {
		if !st.AutoPaused || j.PostMortem {
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

func (s *Scheduler) input(ctx context.Context, j Job, card body.Card) (Input, error) {
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
	return Input{Event: j.Event, Request: j.Request, State: string(b), Episodes: episodes, Places: places, Semantic: semantic}, err
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
			st.AutoPaused = false
			newPlayer = true
			eps = append(eps, memory.Episode{Kind: "request_accepted", Detail: key + ": " + clip(message, 2000)})
		case "death":
			death = true
			st.AutoPaused = true
			s.enqueue(st, "death", true)
			eps = append(eps, memory.Episode{Kind: "death", Detail: e.Text})
		}
	}
	st.Cursor = page.LatestSeq
	if len(page.Events) > 0 {
		st.CursorTS = page.Events[len(page.Events)-1].TS
	}
	pc := judge.NewPrechecker(judge.DefaultConfig().Prechecks)
	danger := pc.Check(*card).Decision == judge.DecideDanger
	if danger && !st.Danger {
		st.AutoPaused = true
		s.enqueue(st, "judge_interrupt", true)
		eps = append(eps, memory.Episode{Kind: "danger", Detail: "code prechecks paused scheduling"})
	} else if !danger && st.Danger {
		st.AutoPaused = false
		eps = append(eps, memory.Episode{Kind: "danger_cleared", Detail: "live prechecks safe; queued goals eligible"})
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
		// Post-mortems curate lessons but cannot start another failure/retry loop.
		if j.PostMortem {
			d.Goal = nil
			d.Idle = true
		}
		j.Goal = d.Goal
		j.Status = "queued"
		sendReply := j.Lane == "player" && !j.ReplySent
		if sendReply {
			j.ReplySent = true // persist intent BEFORE network I/O: at-most-once attempt
		}
		job := *j
		if d.Goal == nil {
			// Persist terminal idle/refusal along with send intent. A crash after
			// chat must not make the answered request eligible for replanning.
			eps = append(eps, memory.Episode{Kind: "idle", Detail: job.ID + ": " + d.Reply})
			removeJob(st, r.id)
		}
		if sendReply {
			if err := s.Memory.Commit(ctx, *st, eps); err != nil {
				return err
			}
			eps = nil
			reply := d.Reply
			if reply == "" {
				if d.Idle {
					reply = "I don't have a supported goal for that request."
				} else {
					reply = "I'll work on that now."
				}
			}
			_, err := s.World.Action(ctx, map[string]any{"type": "chat", "message": reply})
			eps = append(eps, memory.Episode{Kind: "reply_attempt", Detail: fmt.Sprintf("%s latency_ms=%d reply=%s error=%v", job.ID, time.Since(job.Accepted).Milliseconds(), reply, err)})
		}
	} else {
		status := r.out.Outcome
		if r.err != nil {
			status = executor.Failed
		}
		eps = append(eps, memory.Episode{Kind: "goal_" + status, Detail: fmt.Sprintf("%s %s: %s %s error=%v", j.ID, j.Goal.Description, r.out.Detail, r.out.Until, r.err), Tokens: r.out.Tokens.Total, CostUSD: r.out.CostUSD})
		if preempt != "" && status != executor.Succeeded && preempt != "death" {
			j.Status = "queued"
			eps = append(eps, memory.Episode{Kind: "requeued", Detail: j.ID + ": " + preempt})
		} else {
			lane := j.Lane
			removeJob(st, r.id)
			if status == executor.Succeeded {
				if lane == "autonomous" {
					st.AutonomousRuns++
				}
				s.enqueue(st, "goal_finished", false)
			} else {
				st.AutoPaused = true
				event := "goal_failed"
				if r.out.Interrupt != nil {
					event = "judge_interrupt"
				}
				s.enqueue(st, event, true)
			}
		}
	}
	for _, e := range eps {
		fmt.Fprintf(s.Log, "%s: %s tokens=%d cost_usd=%.6f\n", e.Kind, e.Detail, e.Tokens, e.CostUSD)
	}
	return s.Memory.Commit(ctx, *st, eps)
}
