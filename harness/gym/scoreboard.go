package gym

import (
	"fmt"
	"strings"
	"time"
)

// Scoreboard renders results as a Markdown table with a summary line.
func Scoreboard(agent string, results []Result) string {
	passed := 0
	var b strings.Builder
	b.WriteString("| task | result | time to pass | timeout | agent exit | detail |\n")
	b.WriteString("|------|--------|-------------:|--------:|------------|--------|\n")
	for _, r := range results {
		ttp := "–"
		if r.Result == "pass" {
			passed++
			ttp = fmt.Sprintf("%.1fs", r.Seconds)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n",
			r.Task, r.Result, ttp, time.Duration(r.Timeout), r.AgentExit, strings.ReplaceAll(r.Detail, "|", `\|`))
	}
	return fmt.Sprintf("agent `%s`: %d/%d passed\n\n%s", agent, passed, len(results), b.String())
}
