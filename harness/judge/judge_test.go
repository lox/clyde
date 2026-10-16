package judge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func loadTestQuestions(t *testing.T) map[string]Question {
	t.Helper()
	qs, err := LoadQuestions("../../prompts/judge.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return qs
}

func TestLoadQuestions(t *testing.T) {
	qs := loadTestQuestions(t)
	want := map[string]string{"danger": "noul", "stuck": "noul", "progress": "score", "interrupt": "choice"}
	for id, typ := range want {
		if qs[id].Type != typ {
			t.Errorf("%s type = %q, want %q", id, qs[id].Type, typ)
		}
	}
	// The set must survive the trip to Jev's wire format.
	data, err := json.Marshal(qs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"criteria":{"false":`) {
		t.Errorf("noul criteria did not encode as a true/false map: %s", data)
	}
}

func f(v float64) *float64 { return &v }

func TestDecideConfidenceGate(t *testing.T) {
	cfg := DecisionConfig{MinConfidence: 0.5, Corroborate: 0.7}
	choice := func(c string, conf float64) Answer { return Answer{Type: "choice", Choice: c, Confidence: f(conf)} }
	cases := []struct {
		name    string
		answers map[string]Answer
		want    string
	}{
		{"confident danger", map[string]Answer{"interrupt": choice("danger", 0.5)}, "danger"},
		{"unsure danger is suppressed", map[string]Answer{"interrupt": choice("danger", 0.49), "danger": {Noul: f(0.69)}}, "none"},
		{"unsure danger corroborated by noul", map[string]Answer{"interrupt": choice("danger", 0.3), "danger": {Noul: f(0.7)}}, "danger"},
		{"unsure player_request has no noul", map[string]Answer{"interrupt": choice("player_request", 0.4)}, "none"},
		{"none", map[string]Answer{"interrupt": choice("none", 0.1)}, "none"},
		{"missing answer", map[string]Answer{}, "none"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Decide(tc.answers, cfg); got.Reason != tc.want || got.Source != SourceModel {
				t.Fatalf("Decide = %+v, want reason %q from model", got, tc.want)
			}
		})
	}
}

// A replay run over the fixtures writes one complete record per card and
// emits the labelled interrupts, asking the backend only when pre-checks
// cannot decide.
func TestReplayRunRecords(t *testing.T) {
	cases, err := LoadCases("testdata/cards", "testdata/labels.yaml")
	if err != nil {
		t.Fatal(err)
	}
	backend, err := LoadReplay("testdata/replay.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	fixtures := make([]Fixture, len(cases))
	for i, c := range cases {
		fixtures[i] = c.Fixture
	}
	cfg := DefaultConfig()
	cfg.Rate = 1000
	j := New(&FixtureSource{Fixtures: fixtures}, backend, loadTestQuestions(t), cfg)

	var log bytes.Buffer
	var emitted []Interrupt
	if err := j.Run(context.Background(), &log, func(it Interrupt) { emitted = append(emitted, it) }); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(log.String()), "\n")
	if len(lines) != len(cases) {
		t.Fatalf("%d records, want %d", len(lines), len(cases))
	}
	wantEmitted := 0
	for i, line := range lines {
		c := cases[i]
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			t.Fatalf("%s: record is not JSON: %v", c.Name, err)
		}
		for _, k := range []string{"ts", "card", "prechecks", "interrupt", "confidence_cutoff", "backend", "latency_ms", "tick_ms", "cost_usd"} {
			if _, ok := raw[k]; !ok {
				t.Errorf("%s: record missing %q", c.Name, k)
			}
		}
		var rec Record
		json.Unmarshal([]byte(line), &rec)
		if rec.Backend != "replay" || rec.Error != "" {
			t.Errorf("%s: backend %q error %q", c.Name, rec.Backend, rec.Error)
		}
		var card, fixture any
		json.Unmarshal(rec.Card, &card)
		json.Unmarshal(c.Card, &fixture)
		if !jsonEqual(card, fixture) {
			t.Errorf("%s: logged card differs from the fixture", c.Name)
		}
		if rec.Interrupt.Reason != c.Label.Interrupt {
			t.Errorf("%s: interrupt %+v, want %q", c.Name, rec.Interrupt, c.Label.Interrupt)
		}
		if rec.Interrupt.Reason != NoInterrupt {
			wantEmitted++
		}
		asked := rec.Prechecks.Decision == ""
		if rec.Interrupt.Source != map[bool]string{true: SourceModel, false: SourcePrecheck}[asked] {
			t.Errorf("%s: source %q with pre-check decision %q", c.Name, rec.Interrupt.Source, rec.Prechecks.Decision)
		}
		if asked {
			if len(rec.Questions) != 4 || len(rec.Answers) != 4 || rec.Usage == nil || rec.Usage.InputTokens == 0 || rec.CostUSD <= 0 || rec.Model == "" {
				t.Errorf("%s: model tick missing questions/answers/usage/cost: %s", c.Name, line)
			}
		} else if rec.Questions != nil || rec.Answers != nil {
			t.Errorf("%s: pre-check tick should not ask the model", c.Name)
		}
	}
	if len(emitted) != wantEmitted {
		t.Errorf("emitted %d interrupts, want %d", len(emitted), wantEmitted)
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

// endless yields the same busy card forever, as a body would.
type endless struct{ ts int64 }

func (s *endless) Card(ctx context.Context) (json.RawMessage, error) {
	s.ts += 100
	return json.Marshal(map[string]any{"ts": s.ts, "bot": map[string]any{"health": 20, "oxygen": 20}, "action": map[string]any{"id": 1, "type": "goto", "status": "running"}})
}

// slowBackend sleeps for delay (or until the context ends) and tracks how
// many calls overlap.
type slowBackend struct {
	delay           time.Duration
	active, maxSeen atomic.Int32
	calls           atomic.Int32
}

func (b *slowBackend) Name() string { return "slow" }

func (b *slowBackend) Ask(ctx context.Context, card json.RawMessage, qs map[string]Question) (Result, error) {
	b.calls.Add(1)
	n := b.active.Add(1)
	defer b.active.Add(-1)
	if n > b.maxSeen.Load() {
		b.maxSeen.Store(n)
	}
	start := time.Now()
	select {
	case <-time.After(b.delay):
		return Result{Answers: map[string]Answer{"interrupt": {Type: "choice", Choice: "none", Confidence: f(1)}}, Latency: time.Since(start)}, nil
	case <-ctx.Done():
		return Result{Latency: time.Since(start)}, ctx.Err()
	}
}

func TestTickTimeoutFallsBack(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Deadline = 50 * time.Millisecond
	j := New(&endless{}, &slowBackend{delay: time.Hour}, loadTestQuestions(t), cfg)
	start := time.Now()
	rec, err := j.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("tick ignored its deadline")
	}
	if !rec.Timeout || !strings.HasPrefix(rec.Error, "backend:") {
		t.Fatalf("timeout %v error %q, want a logged backend timeout", rec.Timeout, rec.Error)
	}
	if rec.Interrupt.Reason != NoInterrupt || rec.Interrupt.Source != SourceFallback {
		t.Fatalf("interrupt = %+v, want fallback none", rec.Interrupt)
	}
	if rec.LatencyMs < 50 || rec.Card == nil || rec.Prechecks == nil {
		t.Fatalf("record incomplete: %+v", rec)
	}
}

func TestTickBackendErrorFallsBackToPrechecks(t *testing.T) {
	j := New(&endless{}, errBackend{}, loadTestQuestions(t), DefaultConfig())
	rec, _ := j.Tick(context.Background())
	if rec.Timeout || rec.Error != "backend: boom" || rec.Interrupt.Source != SourceFallback {
		t.Fatalf("record = %+v", rec)
	}
}

type errBackend struct{}

func (errBackend) Name() string { return "err" }
func (errBackend) Ask(context.Context, json.RawMessage, map[string]Question) (Result, error) {
	return Result{}, errors.New("boom")
}

// Ticks run serially; when a tick overruns its slot the next one starts
// late instead of a backlog of ticks firing back to back.
func TestRunNeverOverlapsOrQueues(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Rate = 50 // 20 ms slots
	backend := &slowBackend{delay: 100 * time.Millisecond}
	j := New(&endless{}, backend, loadTestQuestions(t), cfg)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var log bytes.Buffer
	if err := j.Run(ctx, &log, nil); err != nil {
		t.Fatal(err)
	}
	if backend.maxSeen.Load() != 1 {
		t.Fatalf("max concurrent backend calls = %d, want 1", backend.maxSeen.Load())
	}
	// 1 s of 100 ms ticks is about 10; queued slots would push it toward 50.
	if n := backend.calls.Load(); n < 5 || n > 12 {
		t.Fatalf("%d ticks in 1s with 100ms calls, want about 10", n)
	}
	if got := strings.Count(log.String(), "\n"); got < 5 {
		t.Fatalf("%d records logged", got)
	}
}

func TestEvalWithReplay(t *testing.T) {
	cases, err := LoadCases("testdata/cards", "testdata/labels.yaml")
	if err != nil {
		t.Fatal(err)
	}
	backend, err := LoadReplay("testdata/replay.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	r, err := Eval(context.Background(), backend, loadTestQuestions(t), DefaultConfig(), cases, 2, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Calls != 2*len(cases) || r.Errors != 0 || len(r.Latencies) != r.Calls {
		t.Fatalf("calls %d errors %d latencies %d", r.Calls, r.Errors, len(r.Latencies))
	}
	if j := r.Metrics["judge"]; j[1] != r.Calls {
		t.Fatalf("judge scored %d of %d calls", j[1], r.Calls)
	}
	var out bytes.Buffer
	r.Print(&out)
	for _, want := range []string{"danger", "interrupt", "judge", "latency p50", "p95"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report missing %q:\n%s", want, out.String())
		}
	}
}

func TestPercentile(t *testing.T) {
	d := make([]time.Duration, 20)
	for i := range d {
		d[i] = time.Duration(i+1) * time.Millisecond
	}
	if p := Percentile(d, 50); p != 10*time.Millisecond {
		t.Errorf("p50 = %v", p)
	}
	if p := Percentile(d, 95); p != 19*time.Millisecond {
		t.Errorf("p95 = %v", p)
	}
}
