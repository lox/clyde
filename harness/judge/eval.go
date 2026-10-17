package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/lox/clyde/harness/body"
)

// Label is the expected judgement for a fixture card. Nil fields are not
// scored for that card.
type Label struct {
	Danger      *bool  `yaml:"danger"`
	Stuck       *bool  `yaml:"stuck"`
	Progressing *bool  `yaml:"progressing"`
	Interrupt   string `yaml:"interrupt"`
}

// Case is a labelled fixture.
type Case struct {
	Fixture
	Label Label
}

// LoadCases pairs the cards in dir with labels from a YAML map of fixture
// name to Label. Every card must have a label and vice versa.
func LoadCases(dir, labelsPath string) ([]Case, error) {
	fixtures, err := LoadFixtures(dir)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(labelsPath)
	if err != nil {
		return nil, err
	}
	var labels map[string]Label
	if err := yaml.Unmarshal(data, &labels); err != nil {
		return nil, fmt.Errorf("%s: %w", labelsPath, err)
	}
	cases := make([]Case, 0, len(fixtures))
	for _, f := range fixtures {
		l, ok := labels[f.Name]
		if !ok {
			return nil, fmt.Errorf("%s: no label for fixture %q", labelsPath, f.Name)
		}
		if l.Interrupt == "" {
			return nil, fmt.Errorf("%s: fixture %q needs an interrupt label", labelsPath, f.Name)
		}
		delete(labels, f.Name)
		cases = append(cases, Case{Fixture: f, Label: l})
	}
	for name := range labels {
		return nil, fmt.Errorf("%s: label %q has no card in %s", labelsPath, name, dir)
	}
	return cases, nil
}

// EvalReport summarises an eval run.
type EvalReport struct {
	Backend      string
	Calls        int
	Errors       int
	OverDeadline int
	// Metric name -> [correct, scored].
	Metrics     map[string][2]int
	Latencies   []time.Duration
	InputTokens int
	CostUSD     float64
	Misses      []string
}

// Eval asks the backend every question for every case, reps times, scoring
// each question against the labels ("model" accuracy) and the full judge
// verdict, pre-checks first then model, against the interrupt label ("judge"
// accuracy). Unlike a live tick, the model is asked even when pre-checks
// decide, so every call measures the model. If log is non-nil, one Record per
// call is written to it, which makes the log a replay file.
func Eval(ctx context.Context, backend Backend, questions map[string]Question, cfg Config, cases []Case, reps int, callTimeout time.Duration, log io.Writer) (EvalReport, error) {
	r := EvalReport{Backend: backend.Name(), Metrics: map[string][2]int{}}
	var enc *json.Encoder
	if log != nil {
		enc = json.NewEncoder(log)
	}
	score := func(metric string, ok bool, miss string) {
		m := r.Metrics[metric]
		m[1]++
		if ok {
			m[0]++
		} else {
			r.Misses = append(r.Misses, miss)
		}
		r.Metrics[metric] = m
	}
	for rep := 1; rep <= reps; rep++ {
		for _, c := range cases {
			var card body.Card
			if err := json.Unmarshal(c.Card, &card); err != nil {
				return r, fmt.Errorf("%s: %w", c.Name, err)
			}
			pre := NewPrechecker(cfg.Prechecks).Check(card)

			callCtx, cancel := context.WithTimeout(ctx, callTimeout)
			res, err := backend.Ask(callCtx, c.Card, questions)
			cancel()
			r.Calls++
			tag := fmt.Sprintf("rep %d %s", rep, c.Name)
			rec := Record{
				TS: time.Now().UTC(), Card: c.Card, Prechecks: &pre, Questions: questions,
				ConfidenceCutoff: cfg.Decision.MinConfidence, Backend: backend.Name(), LatencyMs: ms(res.Latency),
			}
			if err != nil {
				r.Errors++
				r.Misses = append(r.Misses, fmt.Sprintf("%s: error: %v", tag, err))
				rec.Error = err.Error()
				if enc != nil {
					if err := enc.Encode(rec); err != nil {
						return r, err
					}
				}
				continue
			}
			r.Latencies = append(r.Latencies, res.Latency)
			if res.Latency > cfg.Deadline {
				r.OverDeadline++
			}
			r.InputTokens += res.Usage.InputTokens
			cost := float64(res.Usage.InputTokens) * cfg.CostPerMTok / 1e6
			r.CostUSD += cost

			a := res.Answers
			if l := c.Label.Danger; l != nil {
				v := noul(a["danger"])
				score("danger", !math.IsNaN(v) && (v >= 0.5) == *l, fmt.Sprintf("%s: danger noul %.2f, want %v", tag, v, *l))
			}
			if l := c.Label.Stuck; l != nil {
				v := noul(a["stuck"])
				score("stuck", !math.IsNaN(v) && (v >= 0.5) == *l, fmt.Sprintf("%s: stuck noul %.2f, want %v", tag, v, *l))
			}
			if l := c.Label.Progressing; l != nil {
				v, mid := scoreValue(a["progress"]), midLevel(questions["progress"])
				score("progress", !math.IsNaN(v) && (v >= mid) == *l, fmt.Sprintf("%s: progress score %.2f (mid %.1f), want progressing=%v", tag, v, mid, *l))
			}
			choice := a[InterruptQuestion]
			score("interrupt", choice.Choice == c.Label.Interrupt,
				fmt.Sprintf("%s: interrupt choice %q (confidence %.2f), want %q", tag, choice.Choice, deref(choice.Confidence), c.Label.Interrupt))

			verdict, ok := FromPrechecks(pre)
			if !ok {
				verdict = Decide(a, cfg.Decision)
			}
			verdict.CardTS = int64(card.Ts)
			score("judge", verdict.Reason == c.Label.Interrupt,
				fmt.Sprintf("%s: judge verdict %q via %s (%s), want %q", tag, verdict.Reason, verdict.Source, verdict.Detail, c.Label.Interrupt))

			rec.Model, rec.Answers, rec.Usage, rec.CostUSD, rec.Interrupt = res.Model, a, &res.Usage, cost, verdict
			if enc != nil {
				if err := enc.Encode(rec); err != nil {
					return r, err
				}
			}
		}
	}
	return r, nil
}

// Print writes the report as plain text.
func (r EvalReport) Print(w io.Writer) {
	fmt.Fprintf(w, "backend %s: %d calls, %d errors\n", r.Backend, r.Calls, r.Errors)
	var modelOK, modelN int
	for _, name := range []string{"danger", "stuck", "progress", "interrupt", "judge"} {
		m, ok := r.Metrics[name]
		if !ok {
			continue
		}
		label := "model"
		if name == "judge" {
			label = "judge (pre-checks + model)"
		} else {
			modelOK += m[0]
			modelN += m[1]
		}
		fmt.Fprintf(w, "  %-10s %3d/%-3d %5.1f%%  %s\n", name, m[0], m[1], pct(m[0], m[1]), label)
	}
	fmt.Fprintf(w, "  %-10s %3d/%-3d %5.1f%%  all model questions\n", "overall", modelOK, modelN, pct(modelOK, modelN))
	if n := len(r.Latencies); n > 0 {
		lat := slices.Clone(r.Latencies)
		slices.Sort(lat)
		fmt.Fprintf(w, "latency p50 %s  p95 %s  max %s  (%d over deadline)\n",
			Percentile(lat, 50).Round(time.Millisecond), Percentile(lat, 95).Round(time.Millisecond), lat[n-1].Round(time.Millisecond), r.OverDeadline)
		fmt.Fprintf(w, "tokens %d input avg/call  cost $%.6f total, $%.7f/call\n", r.InputTokens/n, r.CostUSD, r.CostUSD/float64(n))
	}
	if len(r.Misses) > 0 {
		fmt.Fprintf(w, "misses:\n  %s\n", strings.Join(r.Misses, "\n  "))
	}
}

// Percentile returns the nearest-rank percentile of sorted durations.
func Percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	i := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	return sorted[max(0, min(i, len(sorted)-1))]
}

func noul(a Answer) float64       { return deref(a.Noul) }
func scoreValue(a Answer) float64 { return deref(a.Score) }

func deref(f *float64) float64 {
	if f == nil {
		return math.NaN()
	}
	return *f
}

// midLevel is the middle of a score question's levels: above it counts as
// progressing.
func midLevel(q Question) float64 {
	l, _ := q.Criteria.([]any)
	return float64(len(l)-1) / 2
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}
