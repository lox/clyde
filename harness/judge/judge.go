// Package judge is Clyde's tier-1 System-1 loop. Each tick it reads the state
// card, runs code pre-checks, asks a backend a fixed set of typed questions
// only when code cannot decide, emits an interrupt reason, and logs the tick.
// It never chooses actions.
package judge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lox/clyde/harness/body"
	"github.com/lox/clyde/harness/goal"
)

// Interrupt sources.
const (
	SourcePrecheck = "precheck"
	SourceModel    = "model"
	SourceFallback = "fallback"
)

// Interrupt is the judge's verdict for one tick: a reason for the executor to
// stop and replan, or "none".
type Interrupt struct {
	CardTS      int64    `json:"card_ts"`
	ActionToken string   `json:"action_token,omitempty"` // only a live observed owner
	Reason      string   `json:"reason"`
	Source      string   `json:"source"`
	Confidence  *float64 `json:"confidence,omitempty"`
	Detail      string   `json:"detail,omitempty"`
}

// Record is one JSONL line per tick. It keeps the full card and full answers
// because these logs are the training data for a distilled judge.
type Record struct {
	TS               time.Time           `json:"ts"`
	Card             json.RawMessage     `json:"card,omitempty"`
	Observation      json.RawMessage     `json:"observation,omitempty"` // exact backend input, including factual judge context
	Prechecks        *Prechecks          `json:"prechecks,omitempty"`
	Questions        map[string]Question `json:"questions,omitempty"`
	Answers          map[string]Answer   `json:"answers,omitempty"`
	Interrupt        Interrupt           `json:"interrupt"`
	ConfidenceCutoff float64             `json:"confidence_cutoff"`
	Backend          string              `json:"backend"`
	Model            string              `json:"model,omitempty"`
	LatencyMs        float64             `json:"latency_ms"`
	StateMs          float64             `json:"state_ms"`
	TickMs           float64             `json:"tick_ms"`
	Usage            *Usage              `json:"usage,omitempty"`
	CostUSD          float64             `json:"cost_usd"`
	Timeout          bool                `json:"timeout,omitempty"`
	Error            string              `json:"error,omitempty"`
}

// Source yields state cards. It returns io.EOF when there are no more.
type Source interface {
	Card(ctx context.Context) (json.RawMessage, error)
}

// Judge runs ticks. It is not safe for concurrent use; Run ticks serially.
type Judge struct {
	src       Source
	backend   Backend
	questions map[string]Question
	cfg       Config
	pre       *Prechecker
	recovery  *goal.Goal
	danger    Danger
	previous  *body.Card
}

// New returns a judge with fresh pre-check state.
func New(src Source, backend Backend, questions map[string]Question, cfg Config) *Judge {
	return &Judge{src: src, backend: backend, questions: questions, cfg: cfg, pre: NewPrechecker(cfg.Prechecks)}
}

// Recovering leaves danger assessment to the active model for this goal.
// Deterministic danger checks supply evidence, not a veto over their response.
// The existing stall guard, caller cancellation and body safety still apply.
func (j *Judge) Recovering(g goal.Goal, baseline Danger) {
	j.recovery, j.danger = &g, baseline
}

// Tick runs one tick under the configured deadline. It returns io.EOF when
// the source is exhausted; every other failure is recorded in the Record.
func (j *Judge) Tick(ctx context.Context) (Record, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, j.cfg.Deadline)
	defer cancel()

	rec := Record{TS: start.UTC(), Backend: j.backend.Name(), ConfidenceCutoff: j.cfg.Decision.MinConfidence}
	var card body.Card
	finish := func() (Record, error) {
		if rec.Interrupt.Reason == "stuck" {
			if card.Action == nil || card.Action.Status != body.ActionStatusStatusRunning || card.Action.Token == "" {
				rec.Interrupt.Reason, rec.Interrupt.Detail = NoInterrupt, "inapplicable stuck: no live action owner"
			} else {
				rec.Interrupt.ActionToken = card.Action.Token
			}
		}
		rec.TickMs = ms(time.Since(start))
		return rec, nil
	}

	raw, err := j.src.Card(ctx)
	rec.StateMs = ms(time.Since(start))
	if errors.Is(err, io.EOF) {
		return rec, io.EOF
	}
	if err != nil {
		rec.Error = "state: " + err.Error()
		rec.Timeout = errors.Is(err, context.DeadlineExceeded)
		rec.Interrupt = Interrupt{Reason: NoInterrupt, Source: SourceFallback, Detail: "no state card"}
		return finish()
	}
	rec.Card = raw
	if err := json.Unmarshal(raw, &card); err != nil {
		rec.Error = "decode card: " + err.Error()
		rec.Interrupt = Interrupt{Reason: NoInterrupt, Source: SourceFallback, Detail: "undecodable state card"}
		return finish()
	}
	previousCard := j.previous
	if previousCard != nil && card.Ts < previousCard.Ts {
		previousCard, j.danger = nil, nil
	}
	j.previous = &card
	pre := j.pre.Check(card)
	rec.Prechecks = &pre
	// Observations stay judge-owned. Ordinary clients keep their raw card;
	// recoveries additionally receive factual preceding observations.
	if j.recovery != nil {
		var observation map[string]json.RawMessage
		if err := json.Unmarshal(raw, &observation); err != nil {
			return rec, err
		}
		current := dangers(pre)
		previous := j.danger
		j.danger = current
		pre.Decision = ""
		for _, c := range pre.Checks {
			if c.Name == "stalled" && c.Fired {
				pre.Decision = DecideStuck
			}
		}
		observation["recovery"], _ = json.Marshal(map[string]any{
			"goal": j.recovery, "previous": previous, "current": current,
			"appeared": current.difference(previous), "cleared": previous.difference(current),
			"previous_card": previousCard, "damage_cause": "unknown",
		})
		raw, err = json.Marshal(observation)
		if err != nil {
			return rec, err
		}
	}
	rec.Observation = raw
	if it, ok := FromPrechecks(pre); ok {
		it.CardTS = int64(card.Ts)
		rec.Interrupt = it
		return finish()
	}

	rec.Questions = j.questions
	res, err := j.backend.Ask(ctx, raw, j.questions)
	rec.LatencyMs = ms(res.Latency)
	if err != nil {
		rec.Error = "backend: " + err.Error()
		rec.Timeout = errors.Is(err, context.DeadlineExceeded)
		rec.Interrupt = Interrupt{CardTS: int64(card.Ts), Reason: NoInterrupt, Source: SourceFallback, Detail: "backend failed; no deterministic interrupt"}
		return finish()
	}
	rec.Model = res.Model
	rec.Answers = res.Answers
	rec.Usage = &res.Usage
	rec.CostUSD = float64(res.Usage.InputTokens) * j.cfg.CostPerMTok / 1e6
	rec.Interrupt = Decide(res.Answers, j.cfg.Decision)
	rec.Interrupt.CardTS = int64(card.Ts)
	return finish()
}

// Run ticks at the configured rate until ctx ends or the source is exhausted, writing
// one JSON line per tick to log and calling onInterrupt for every interrupt
// other than "none". Ticks never overlap: an overrunning tick delays the next
// one, and missed slots are dropped rather than queued.
func (j *Judge) Run(ctx context.Context, log io.Writer, onInterrupt func(Interrupt)) error {
	period := time.Duration(float64(time.Second) / j.cfg.Rate)
	enc := json.NewEncoder(log)
	next := time.Now()
	for {
		rec, err := j.Tick(ctx)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if ctx.Err() != nil {
			return nil
		}
		if err := enc.Encode(rec); err != nil {
			return fmt.Errorf("write log: %w", err)
		}
		if rec.Interrupt.Reason != NoInterrupt && onInterrupt != nil {
			onInterrupt(rec.Interrupt)
		}
		next = next.Add(period)
		if now := time.Now(); next.Before(now) {
			next = now
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Until(next)):
		}
	}
}

// FromPrechecks converts a pre-check decision into an interrupt. ok is false
// when code could not decide and the model must be asked.
func FromPrechecks(pre Prechecks) (Interrupt, bool) {
	fired := func() string {
		var names []string
		for _, c := range pre.Checks {
			if c.Fired {
				names = append(names, c.Name)
			}
		}
		return strings.Join(names, ",")
	}
	switch pre.Decision {
	case DecideDanger:
		return Interrupt{Reason: "danger", Source: SourcePrecheck, Detail: fired()}, true
	case DecideStuck:
		return Interrupt{Reason: "stuck", Source: SourcePrecheck, Detail: "stalled"}, true
	case DecideIdle:
		return Interrupt{Reason: NoInterrupt, Source: SourcePrecheck, Detail: "idle"}, true
	}
	return Interrupt{}, false
}

// Decide turns model answers into an interrupt. The interrupt choice wins
// when its confidence is at least MinConfidence; below that it interrupts
// only if the noul question named after the reason is at least Corroborate.
func Decide(answers map[string]Answer, cfg DecisionConfig) Interrupt {
	a, ok := answers[InterruptQuestion]
	if !ok || a.Choice == "" {
		return Interrupt{Reason: NoInterrupt, Source: SourceModel, Detail: "no interrupt answer"}
	}
	it := Interrupt{Reason: a.Choice, Source: SourceModel, Confidence: a.Confidence}
	if a.Choice == NoInterrupt {
		return it
	}
	conf := 0.0
	if a.Confidence != nil {
		conf = *a.Confidence
	}
	if conf >= cfg.MinConfidence {
		return it
	}
	if n, ok := answers[a.Choice]; ok && n.Noul != nil && *n.Noul >= cfg.Corroborate {
		it.Detail = fmt.Sprintf("confidence %.2f < %.2f, corroborated by %s noul %.2f", conf, cfg.MinConfidence, a.Choice, *n.Noul)
		return it
	}
	it.Reason = NoInterrupt
	it.Detail = fmt.Sprintf("suppressed %s: confidence %.2f < %.2f", a.Choice, conf, cfg.MinConfidence)
	return it
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
