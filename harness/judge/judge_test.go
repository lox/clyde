package judge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lox/clyde/harness/body"
	"github.com/lox/clyde/harness/goal"
)

func TestRecoverySendsDangerChangesToModelAndRetainsStallGuard(t *testing.T) {
	b, err := os.ReadFile("testdata/cards/low_health.json")
	if err != nil {
		t.Fatal(err)
	}
	var initial body.Card
	if err := json.Unmarshal(b, &initial); err != nil {
		t.Fatal(err)
	}
	initial.Bot.Health = 5
	cases := []struct {
		name   string
		change func(*body.Card)
		want   string
	}{
		{"persistent", func(*body.Card) {}, "none"},
		{"improving", func(c *body.Card) { c.Bot.Health = 6 }, "none"},
		{"worsening", func(c *body.Card) { c.Bot.Health = 4 }, "none"},
		{"new oxygen danger", func(c *body.Card) { c.Bot.Oxygen = 3 }, "none"},
		{"new lava danger", func(c *body.Card) { c.Bot.InLava = true }, "none"},
		{"new hostile", func(c *body.Card) {
			c.Entities.Hostiles = []body.NearbyEntity{{Name: "zombie", Distance: 3, Direction: "N"}}
		}, "none"},
		{"stalled despite danger", func(c *body.Card) { c.Ts += 10_000 }, "stuck"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start := initial
			start.Action = &body.ActionStatus{ID: 1, Type: "eval", Status: "running", Params: map[string]any{}, StartedAt: start.Ts}
			next := start
			next.Ts += 500
			tc.change(&next)
			var fixtures []Fixture
			for _, c := range []body.Card{start, next} {
				raw, e := json.Marshal(c)
				if e != nil {
					t.Fatal(e)
				}
				fixtures = append(fixtures, Fixture{Card: raw})
			}
			backend := &slowBackend{}
			j := New(&FixtureSource{Fixtures: fixtures}, backend, loadTestQuestions(t), DefaultConfig())
			j.Recovering(goal.Goal{Description: "model-selected response"}, Dangers(start, DefaultConfig().Prechecks))
			first, e := j.Tick(context.Background())
			if e != nil || first.Interrupt.Reason != "none" {
				t.Fatalf("first=%+v err=%v", first, e)
			}
			got, e := j.Tick(context.Background())
			if e != nil || got.Interrupt.Reason != tc.want {
				t.Fatalf("got=%+v err=%v want=%s", got, e, tc.want)
			}
			wantCalls := int32(2)
			if tc.want == "stuck" {
				wantCalls = 1
			}
			if backend.calls.Load() != wantCalls {
				t.Fatalf("new evidence did not reach active judge: calls=%d", backend.calls.Load())
			}
			var observation struct {
				Recovery struct {
					Goal         goal.Goal  `json:"goal"`
					Current      Danger     `json:"current"`
					Appeared     Danger     `json:"appeared"`
					PreviousCard *body.Card `json:"previous_card"`
					DamageCause  string     `json:"damage_cause"`
				} `json:"recovery"`
			}
			if err := json.Unmarshal(got.Observation, &observation); err != nil {
				t.Fatal(err)
			}
			r := observation.Recovery
			if r.Goal.Description != "model-selected response" || r.PreviousCard == nil || r.PreviousCard.Bot.Health != 5 || r.DamageCause != "unknown" || r.Current[0].Value != next.Bot.Health {
				t.Fatalf("missing factual recovery context: %+v", r)
			}
			if strings.HasPrefix(tc.name, "new ") && len(r.Appeared) != 1 {
				t.Fatalf("new threat omitted on its tick: %+v", r)
			}
		})
	}
	// Clearing and returning to a condition remains evidence for the model,
	// not a new deterministic veto.
	var fixtures []Fixture
	for i, h := range []float64{5, 20, 5} {
		c := initial
		c.Ts += i * 500
		c.Bot.Health = h
		raw, _ := json.Marshal(c)
		fixtures = append(fixtures, Fixture{Card: raw})
	}
	j := New(&FixtureSource{Fixtures: fixtures}, &slowBackend{}, loadTestQuestions(t), DefaultConfig())
	j.Recovering(goal.Goal{Description: "model-selected response"}, Dangers(initial, DefaultConfig().Prechecks))
	for i, want := range []string{"none", "none", "none"} {
		rec, e := j.Tick(context.Background())
		if e != nil || rec.Interrupt.Reason != want {
			t.Fatalf("tick%d: %+v err=%v", i, rec, e)
		}
	}
}

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

// busyCard is a running collect, read once so ticks spend no time on disk.
var busyCard = func() map[string]any {
	b, err := os.ReadFile("testdata/cards/collect_progressing.json")
	if err != nil {
		panic(err)
	}
	var card map[string]any
	if err := json.Unmarshal(b, &card); err != nil {
		panic(err)
	}
	return card
}()

func (s *endless) Card(ctx context.Context) (json.RawMessage, error) {
	s.ts += 100
	card := map[string]any{}
	for k, v := range busyCard {
		card[k] = v
	}
	card["ts"] = s.ts
	return json.Marshal(card)
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
	// The deadline covers observation/decoding too, not 50ms of backend time.
	if rec.TickMs < 45 || rec.Card == nil || rec.Prechecks == nil {
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
