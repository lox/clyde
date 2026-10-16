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
	CardTS     int64    `json:"card_ts"`
	Reason     string   `json:"reason"`
	Source     string   `json:"source"`
	Confidence *float64 `json:"confidence,omitempty"`
	Detail     string   `json:"detail,omitempty"`
}

// Record is one JSONL line per tick. It keeps the full card and full answers
// because these logs are the training data for a distilled judge.
type Record struct {
	TS               time.Time           `json:"ts"`
	Card             json.RawMessage     `json:"card,omitempty"`
	Prechecks        *Prechecks          `json:"prechecks,omitempty"`
	Questions        map[string]Question `json:"questions,omitempty"`
	Answers          map[string]Answer   `json:"answers,omitempty"`
	Interrupt        Interrupt           `json:"interrupt"`
	ConfidenceCutoff float64             `json:"confidence_cutoff"`
	Backend          string              `json:"backend"`
	Model            string              `json:"model,omitempty"`
	LatencyMs        float64             `json:"latency_ms"`
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
}

// New returns a judge with fresh pre-check state.
func New(src Source, backend Backend, questions map[string]Question, cfg Config) *Judge {
	return &Judge{src: src, backend: backend, questions: questions, cfg: cfg, pre: NewPrechecker(cfg.Prechecks)}
}

// Tick runs one tick under the configured deadline. It returns io.EOF when
// the source is exhausted; every other failure is recorded in the Record.
func (j *Judge) Tick(ctx context.Context) (Record, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, j.cfg.Deadline)
	defer cancel()

	rec := Record{TS: start.UTC(), Backend: j.backend.Name(), ConfidenceCutoff: j.cfg.Decision.MinConfidence}
	finish := func() (Record, error) {
		rec.TickMs = ms(time.Since(start))
		return rec, nil
	}

	raw, err := j.src.Card(ctx)
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
	var card Card
	if err := json.Unmarshal(raw, &card); err != nil {
		rec.Error = "decode card: " + err.Error()
		rec.Interrupt = Interrupt{Reason: NoInterrupt, Source: SourceFallback, Detail: "undecodable state card"}
		return finish()
	}
	pre := j.pre.Check(card)
	rec.Prechecks = &pre
	if it, ok := FromPrechecks(pre); ok {
		it.CardTS = card.TS
		rec.Interrupt = it
		return finish()
	}

	rec.Questions = j.questions
	res, err := j.backend.Ask(ctx, raw, j.questions)
	rec.LatencyMs = ms(res.Latency)
	if err != nil {
		rec.Error = "backend: " + err.Error()
		rec.Timeout = errors.Is(err, context.DeadlineExceeded)
		rec.Interrupt = Interrupt{CardTS: card.TS, Reason: NoInterrupt, Source: SourceFallback, Detail: "backend failed; pre-checks did not fire"}
		return finish()
	}
	rec.Model = res.Model
	rec.Answers = res.Answers
	rec.Usage = &res.Usage
	rec.CostUSD = float64(res.Usage.InputTokens) * j.cfg.CostPerMTok / 1e6
	rec.Interrupt = Decide(res.Answers, j.cfg.Decision)
	rec.Interrupt.CardTS = card.TS
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
