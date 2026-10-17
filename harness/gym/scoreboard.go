package gym

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Scoreboard renders results as Markdown: a summary line, one row per task
// (pass rate, median time to pass, and mean tokens and cost over the runs
// whose agent reported them), and one row per run.
func Scoreboard(agent string, results []Result) string {
	type agg struct {
		runs, passed int
		reported     int // runs whose agent reported usage
		times        []float64
		tokens       int
		cost         float64
		models       []string
	}
	var order []string
	byTask := map[string]*agg{}
	passed := 0
	for _, r := range results {
		a := byTask[r.Task]
		if a == nil {
			a = &agg{}
			byTask[r.Task] = a
			order = append(order, r.Task)
		}
		a.runs++
		if r.Tokens > 0 || r.CostUSD > 0 {
			a.reported++
		}
		a.tokens += r.Tokens
		a.cost += r.CostUSD
		if r.Model != "" && !slices.Contains(a.models, r.Model) {
			a.models = append(a.models, r.Model)
		}
		if r.Result == "pass" {
			passed++
			a.passed++
			a.times = append(a.times, r.Seconds)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "agent `%s`: %d/%d passed\n\n", agent, passed, len(results))
	b.WriteString("| task | passed | median time to pass | tokens/run | cost/run | model |\n")
	b.WriteString("|------|-------:|--------------------:|-----------:|---------:|-------|\n")
	for _, task := range order {
		a := byTask[task]
		fmt.Fprintf(&b, "| %s | %d/%d | %s | %s | %s | %s |\n", task, a.passed, a.runs, median(a.times),
			orDash(a.tokens > 0, fmt.Sprintf("%d", a.tokens/max(a.reported, 1))), orDash(a.cost > 0, fmt.Sprintf("$%.4f", a.cost/float64(max(a.reported, 1)))),
			orDash(len(a.models) > 0, strings.Join(a.models, ", ")))
	}

	b.WriteString("\n| task | run | result | time to pass | timeout | agent exit | tokens | cost | detail |\n")
	b.WriteString("|------|----:|--------|-------------:|--------:|------------|-------:|-----:|--------|\n")
	for _, r := range results {
		ttp := "–"
		if r.Result == "pass" {
			ttp = fmt.Sprintf("%.1fs", r.Seconds)
		}
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %s | %s | %s | %s | %s |\n",
			r.Task, max(r.Rep, 1), r.Result, ttp, time.Duration(r.Timeout), r.AgentExit,
			orDash(r.Tokens > 0, fmt.Sprintf("%d", r.Tokens)), orDash(r.CostUSD > 0, fmt.Sprintf("$%.4f", r.CostUSD)),
			strings.ReplaceAll(r.Detail, "|", `\|`))
	}
	return b.String()
}

func median(xs []float64) string {
	if len(xs) == 0 {
		return "–"
	}
	s := slices.Sorted(slices.Values(xs))
	m := s[len(s)/2]
	if len(s)%2 == 0 {
		m = (s[len(s)/2-1] + s[len(s)/2]) / 2
	}
	return fmt.Sprintf("%.1fs", m)
}

func orDash(ok bool, s string) string {
	if !ok {
		return "–"
	}
	return s
}
