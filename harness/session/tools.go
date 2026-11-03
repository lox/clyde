package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	agent "github.com/lox/agent-harness"
	"github.com/lox/clyde/harness/executor"
	"github.com/lox/clyde/harness/goal"
	"github.com/lox/clyde/harness/planner"
)

func (p *Player) tools(s *Snapshot) []executor.Tool {
	tb := &executor.Toolbox{Body: p.Body, SkillsDir: p.SkillsDir, Immediate: true}
	tools := tb.Tools()
	for i := range tools {
		if tools[i].Name == "act" || tools[i].Name == "eval_js" || tools[i].Name == "run_skill" {
			tools[i].Description += " In this persistent session, returns promptly with an action token/status. Work continues; use wait for outcomes. Chat does not replace work."
		}
		if tools[i].Name == "observe" {
			tools[i].Description = "Read fresh state without replacing work. Does not wait; use wait for events/deadlines."
			tools[i].Run = func(ctx context.Context, _ json.RawMessage) (string, error) { return p.Body.StateText(ctx) }
		}
	}
	tools = append(tools,
		executor.Tool{Name: "wait", Description: "Yield until a new player/world event or an optional wake deadline (seconds, maximum 3600). Zero means events only. No inference while waiting. Does not stop ongoing work.", Schema: json.RawMessage(`{"type":"object","properties":{"seconds":{"type":"number","minimum":0,"maximum":3600},"reason":{"type":"string"}},"required":["reason"]}`), Run: func(_ context.Context, raw json.RawMessage) (string, error) {
			var a struct {
				Seconds float64
				Reason  string
			}
			if err := json.Unmarshal(raw, &a); err != nil {
				return "", err
			}
			if a.Seconds < 0 || a.Seconds > 3600 || len(a.Reason) > 2000 {
				return "", errors.New("invalid wait")
			}
			w := &Wait{Reason: a.Reason}
			if a.Seconds > 0 {
				w.Until = time.Now().UTC().Add(time.Duration(a.Seconds * float64(time.Second)))
			}
			s.State.Wait = w
			return "Waiting; current action remains owned and can finish. New events may wake the session.", nil
		}},
		executor.Tool{Name: "verify", Description: "Independently check a milestone using the goal schema. True is evidence, not session termination. Built/delivered need RCON; judged is unsupported.", Schema: p.GoalSchema, Run: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var g goal.Goal
			if err := json.Unmarshal(raw, &g); err != nil {
				return "", err
			}
			result, err := p.Check(ctx, g)
			if err != nil {
				return "", err
			}
			e, err := entry("milestone", struct {
				Goal   goal.Goal   `json:"goal"`
				Result goal.Result `json:"result"`
			}{g, result})
			if err != nil {
				return "", err
			}
			if err := p.save(ctx, s, Update{State: s.State, Entries: []Entry{e}}); err != nil {
				return "", err
			}
			b, err := json.Marshal(result)
			return string(b), err
		}},
		executor.Tool{Name: "history", Description: "Search/read archived original session history by text, after ID, and limit (1–32). Native opaque data stays on disk, not in this text view. To read the rest of a truncated record, use after=id-1, limit=1, offset=next_offset (bytes).", Schema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","maxLength":200},"after":{"type":"integer","minimum":0},"limit":{"type":"integer","minimum":1,"maximum":32},"offset":{"type":"integer","minimum":0}}}`), Run: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var a struct {
				Query  string
				After  int64
				Limit  int
				Offset int
			}
			if err := json.Unmarshal(raw, &a); err != nil {
				return "", err
			}
			if len(a.Query) > 200 || a.Offset < 0 || a.After < 0 || a.Limit < 0 {
				return "", errors.New("invalid history query or pagination")
			}
			if a.Limit == 0 {
				a.Limit = 8
			}
			records, err := p.Journal.History(ctx, a.Query, a.After, min(a.Limit, 32))
			if err != nil {
				return "", err
			}
			var out []map[string]any
			total := 0
			for _, e := range records {
				data := e.Data
				if e.Kind == "message" || e.Kind == "compaction_response" {
					var m agent.Message
					if err := json.Unmarshal(data, &m); err != nil {
						return "", err
					}
					m.ProviderData = nil
					m.Thinking = ""
					data, err = json.Marshal(m)
					if err != nil {
						return "", err
					}
				}
				offset := min(a.Offset, len(data))
				text := string(data[offset:])
				truncated := len(text) > 8000
				next := 0
				if truncated {
					text = text[:8000]
					next = offset + len(text)
				}
				if total+len(text) > 24000 {
					break
				}
				total += len(text)
				out = append(out, map[string]any{"id": e.ID, "at": e.At, "kind": e.Kind, "text": text, "truncated": truncated, "next_offset": next})
			}
			b, err := json.Marshal(out)
			return string(b), err
		}},
		executor.Tool{Name: "compact", Description: "Request a model-written working-memory compaction at the next complete exchange. Original history is retained. Costs one request from the same allowance.", Schema: json.RawMessage(`{"type":"object","properties":{}}`), Run: func(_ context.Context, _ json.RawMessage) (string, error) {
			s.State.ForceCompact = true
			return "Compaction requested after this exchange.", nil
		}},
	)
	if p.Memory != nil {
		tools = append(tools, executor.Tool{Name: "memory", Description: "Read legacy semantic notes, observed places and recent episodes. Notes/project intent are unverified; places and episodes retain provenance.", Schema: json.RawMessage(`{"type":"object","properties":{}}`), Run: func(ctx context.Context, _ json.RawMessage) (string, error) {
			notes, err := p.Memory.Semantic()
			if err != nil {
				return "", err
			}
			places, err := p.Memory.Places(ctx)
			if err != nil {
				return "", err
			}
			episodes, err := p.Memory.Recent(ctx, 16)
			if err != nil {
				return "", err
			}
			b, err := json.Marshal(map[string]any{"notes_untrusted": notes, "observed_places": places, "episodes": episodes})
			return string(b), err
		}})
	}
	if p.Candidates != nil {
		tools = append(tools, executor.Tool{Name: "stage_skill", Description: "Stage a reusable skill proposal, never admit or execute it. An operator must run isolated gym gates and explicitly promote it.", Schema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"skill_md":{"type":"string"},"index_js":{"type":"string"},"test_yaml":{"type":"string"}},"required":["name","skill_md","index_js","test_yaml"]}`), Run: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var c planner.Candidate
			if err := json.Unmarshal(raw, &c); err != nil {
				return "", err
			}
			path, err := p.Candidates.Save(ctx, c)
			return fmt.Sprintf("Staged, not admitted: %s", path), err
		}})
	}
	return tools
}
