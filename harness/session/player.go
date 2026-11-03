package session

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	agent "github.com/lox/agent-harness"
	"github.com/lox/clyde/harness/body"
	"github.com/lox/clyde/harness/goal"
	"github.com/lox/clyde/harness/judge"
	"github.com/lox/clyde/harness/llm"
	"github.com/lox/clyde/harness/memory"
	"github.com/lox/clyde/harness/planner"
)

// Player owns one continuing conversation and one body. The entry point must
// hold the local body and memory leases for its entire lifetime.
type Player struct {
	Journal         *Journal
	Body            *body.Client
	Provider        agent.Provider
	Config          llm.Config
	System          string
	SkillsDir       string
	Memory          *memory.Store
	Candidates      *planner.Candidates
	GoalSchema      json.RawMessage
	Check           func(context.Context, goal.Goal) (goal.Result, error)
	Judge           *judge.Judge
	JudgeLog        io.Writer
	Log             io.Writer
	CallTimeout     time.Duration
	PollInterval    time.Duration
	MaxOutputTokens int
	ContextBytes    int
	pre             *judge.Prechecker
}

// Reconcile stops stale body work and observes before resolving crash windows.
// An intent without a receipt is not evidence of success or failure. Preserve
// its assistant message verbatim and balance every outstanding tool call; never
// replay a possibly dispatched mutation or chat. A failed stop/read publishes
// nothing and must prevent startup, rather than granting uncertain ownership.
func (p *Player) Reconcile(ctx context.Context) (Snapshot, error) {
	if p.Journal == nil || p.Body == nil {
		return Snapshot{}, errors.New("session: journal and body required")
	}
	s, err := p.Journal.Load(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := p.Body.Stop(ctx); err != nil {
		return Snapshot{}, err
	}
	observation, err := p.Body.StateText(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	update := Update{State: s.State}
	for _, pending := range s.State.Pending {
		text := "Tool call withheld after restart; not dispatched."
		if pending.Started {
			text = "Tool outcome unknown after restart; dispatch was recorded but no receipt was committed. It may have taken effect. Not replayed; inspect fresh state before deciding what to do."
		}
		m := agent.Message{Role: agent.RoleTool, ToolResult: &agent.ToolResult{ToolCallID: pending.Call.ID, Content: text, IsError: true}}
		update.State.Messages = append(update.State.Messages, m)
		entry, err := entry("message", m)
		if err != nil {
			return Snapshot{}, err
		}
		update.Entries = append(update.Entries, entry)
	}
	if len(s.State.Pending) > 0 || s.State.ActionToken != "" {
		update.State.Wait = nil // interrupted work needs attention, not replay
		m := agent.Message{Role: agent.RoleUser, Content: "Harness restart: stale body work stopped. Outstanding tool results describe uncertainty, not verified progress. Fresh observations are required before continuing."}
		update.State.Messages = append(update.State.Messages, m)
		entry, err := entry("message", m)
		if err != nil {
			return Snapshot{}, err
		}
		update.Entries = append(update.Entries, entry)
	}
	update.State.Pending = nil
	update.State.ActionToken = ""
	evidence, err := entry("restart_observation", observation)
	if err != nil {
		return Snapshot{}, err
	}
	update.Entries = append(update.Entries, evidence)
	return p.Journal.Commit(ctx, s.Revision, update)
}

func entry(kind string, data any) (Entry, error) {
	raw, err := json.Marshal(data)
	return Entry{At: time.Now().UTC(), Kind: kind, Data: raw}, err
}
