package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agent "github.com/lox/agent-harness"
)

func (p *Player) compact(ctx context.Context, s *Snapshot) error {
	// Preserve the latest entire user/tool exchange, never a suffix of a
	// multi-tool assistant response. Earlier originals are already archived.
	cut := 0
	for i, m := range s.State.Messages {
		if m.Role == agent.RoleUser && i > 0 {
			cut = i
		}
	}
	if cut == 0 {
		s.State.Pause = "context pressure: no complete earlier exchange to compact; operator intervention required"
		s.State.ForceCompact = false
		return p.save(ctx, s, Update{State: s.State})
	}
	request := append([]agent.Message(nil), s.State.Messages...)
	request = append(request, agent.Message{Role: agent.RoleUser, Content: "Write compact working notes (at most 8000 characters) for your continuing session: commitments, requirements and revisions, approach, failed attempts, open questions, useful coordinates, and references to supporting tool call IDs/history. Separate independently verified outcomes from intentions and unknown outcomes. Do not act or invent progress. Return only the notes."})
	result, receipt, stale, err := p.infer(ctx, s, "compaction", p.System, request, nil)
	if receipt == nil {
		if errors.Is(err, ErrAllowance) || errors.Is(err, ErrStopped) {
			return nil
		}
		return err
	}
	u := Update{State: s.State, Receipt: receipt}
	if result != nil {
		e, eerr := entry("compaction_response", result.Message)
		if eerr != nil {
			return eerr
		}
		u.Entries = append(u.Entries, e)
	}
	if err != nil || stale || result == nil || strings.TrimSpace(result.Message.Content) == "" || len(result.Message.Content) > 12000 || len(result.Message.ToolCalls) > 0 {
		u.State.Pause = fmt.Sprintf("compaction failed; history retained, operator resume required (stale=%t error=%v)", stale, err)
	} else {
		note := agent.Message{Role: agent.RoleUser, Content: "Model-written working notes (unverified except where supported by independent history):\n" + result.Message.Content}
		u.State.Messages = append([]agent.Message{note}, s.State.Messages[cut:]...)
		raw, e := json.Marshal(u.State.Messages)
		if e != nil {
			return e
		}
		if len(raw) >= p.ContextBytes {
			u.State = s.State
			u.State.Pause = "compaction could not bound the complete active exchange; operator intervention required"
		} else {
			u.State.Reset = true
		}
	}
	u.State.ForceCompact = false
	return p.save(context.WithoutCancel(ctx), s, u)
}

// Model monitoring is event-driven, once before an inference burst, never a
// paid idle poll. Every attempted tick reserves from the same durable allowance,
// conservatively including ticks the judge handles without a provider call.
func (p *Player) monitor(ctx context.Context, s *Snapshot) error {
	id, err := p.Journal.Reserve(ctx, "monitor", "judge before frontier attention")
	if errors.Is(err, ErrAllowance) || errors.Is(err, ErrStopped) {
		return nil
	}
	if err != nil {
		return err
	}
	rec, err := p.Judge.Tick(ctx)
	if p.JudgeLog != nil {
		if logErr := json.NewEncoder(p.JudgeLog).Encode(rec); logErr != nil {
			return logErr
		}
	}
	receipt := &Receipt{ID: id, CostUSD: rec.CostUSD}
	if rec.Usage != nil {
		receipt.Usage = &agent.Usage{InputTokens: rec.Usage.InputTokens, OutputTokens: rec.Usage.OutputTokens}
	} else if rec.Error == "" && err == nil {
		receipt.Usage = &agent.Usage{}
	}
	e, eerr := entry("judge", rec)
	if eerr != nil {
		return eerr
	}
	u := Update{State: s.State, Receipt: receipt, Entries: []Entry{e}}
	if err == nil && (rec.Interrupt.Reason == "danger" || rec.Interrupt.Reason == "stuck") {
		// Model danger informs the same session; deterministic/body safety owns
		// immediate stops. A stuck verdict can stop only its observed token.
		if rec.Interrupt.Reason == "stuck" && rec.Interrupt.ActionToken != "" {
			if _, err := p.Body.StopAction(ctx, rec.Interrupt.ActionToken); err != nil {
				return err
			}
		}
		m := agent.Message{Role: agent.RoleUser, Content: fmt.Sprintf("Judge evidence, not a new goal or allowance: %s (%s) %s", rec.Interrupt.Reason, rec.Interrupt.Source, rec.Interrupt.Detail)}
		u.State.Messages = append(u.State.Messages, m)
		e, eerr := entry("message", m)
		if eerr != nil {
			return eerr
		}
		u.Entries = append(u.Entries, e)
	}
	// Errors are factual monitoring outcomes; hard prechecks still run locally.
	return p.save(ctx, s, u)
}
