package judge

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/lox/clyde/harness/jev"
)

// Question is one typed question: "noul", "choice", or "score". The shapes
// are Jev's native ones, which every backend must answer.
type Question struct {
	Type         string `json:"type" yaml:"type"`
	Instructions any    `json:"instructions" yaml:"instructions"`
	Criteria     any    `json:"criteria,omitempty" yaml:"criteria"`
}

// Answer is one typed answer; which fields are set depends on Type.
type Answer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
}

// Usage is token usage for one backend call.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Result is what a backend returns for one card.
type Result struct {
	Model   string
	Answers map[string]Answer
	Usage   Usage
	Latency time.Duration
}

// Backend answers questions about a JSON state card, with judge-owned evidence
// added during recovery. Backends must not depend on its Go representation.
type Backend interface {
	Name() string
	Ask(ctx context.Context, card json.RawMessage, questions map[string]Question) (Result, error)
}

// JevBackend asks TypeSafe's Jev through harness/jev.
type JevBackend struct {
	Client *jev.Client
}

func (b JevBackend) Name() string { return "jev" }

func (b JevBackend) Ask(ctx context.Context, card json.RawMessage, questions map[string]Question) (Result, error) {
	qs := make(map[string]jev.Question, len(questions))
	for id, q := range questions {
		qs[id] = jev.Question(q)
	}
	start := time.Now()
	resp, err := b.Client.Evaluate(ctx, card, qs)
	latency := time.Since(start)
	if err != nil {
		return Result{Latency: latency}, err
	}
	answers := make(map[string]Answer, len(resp.Answers))
	for id, a := range resp.Answers {
		answers[id] = Answer(a)
	}
	return Result{Model: resp.Model, Answers: answers, Usage: Usage(resp.Usage), Latency: latency}, nil
}

// ReplayBackend answers from recorded judge JSONL records, matched by the
// card's ts field. Any judge log (including `judge eval --log`) is a valid
// replay file; records without answers are ignored and the first answer for a
// given ts wins.
type ReplayBackend struct {
	byTS map[int64]Record
}

// LoadReplay reads a JSONL file of judge records.
func LoadReplay(path string) (*ReplayBackend, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b := &ReplayBackend{byTS: map[int64]Record{}}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for line := 1; sc.Scan(); line++ {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var rec Record
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		if len(rec.Answers) == 0 {
			continue
		}
		ts, err := cardTS(rec.Card)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		if _, ok := b.byTS[ts]; !ok {
			b.byTS[ts] = rec
		}
	}
	return b, sc.Err()
}

func (b *ReplayBackend) Name() string { return "replay" }

func (b *ReplayBackend) Ask(ctx context.Context, card json.RawMessage, questions map[string]Question) (Result, error) {
	ts, err := cardTS(card)
	if err != nil {
		return Result{}, err
	}
	rec, ok := b.byTS[ts]
	if !ok {
		return Result{}, fmt.Errorf("replay: no recorded answers for card ts %d", ts)
	}
	answers := make(map[string]Answer, len(questions))
	for id := range questions {
		a, ok := rec.Answers[id]
		if !ok {
			return Result{}, fmt.Errorf("replay: card ts %d has no recorded answer for %q", ts, id)
		}
		answers[id] = a
	}
	var usage Usage
	if rec.Usage != nil {
		usage = *rec.Usage
	}
	return Result{
		Model:   rec.Model,
		Answers: answers,
		Usage:   usage,
		Latency: time.Duration(rec.LatencyMs * float64(time.Millisecond)),
	}, nil
}

func cardTS(card json.RawMessage) (int64, error) {
	var c struct {
		TS *int64 `json:"ts"`
	}
	if err := json.Unmarshal(card, &c); err != nil {
		return 0, fmt.Errorf("decode card ts: %w", err)
	}
	if c.TS == nil {
		return 0, fmt.Errorf("card has no ts")
	}
	return *c.TS, nil
}
