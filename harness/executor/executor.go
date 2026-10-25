// Package executor is Clyde's tier-2 loop: given a typed goal, a model calls
// body tools until the goal's until predicate holds, the judge interrupts,
// or the budget runs out. Nothing is carried between goals.
package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	harness "github.com/lox/agent-harness"

	"github.com/lox/clyde/harness/body"
	"github.com/lox/clyde/harness/goal"
	"github.com/lox/clyde/harness/judge"
	"github.com/lox/clyde/harness/llm"
)

// Outcomes.
const (
	Succeeded       = "succeeded"        // the until predicate held
	Failed          = "failed"           // the model gave up, refused, or errored
	Interrupted     = "interrupted"      // the judge raised danger
	BudgetExhausted = "budget_exhausted" // max steps or wall time
	Cancelled       = "cancelled"        // the caller cancelled the run
)

// ExecutionContext is harness-assigned intent, not model authority to bypass
// safety. Its zero value is ordinary work, preserving run and MCP clients.
type ExecutionContext struct {
	Mode      string        `json:"mode,omitempty"` // empty, recovery, danger
	Cause     string        `json:"cause,omitempty"`
	Remaining int           `json:"remaining"`
	Danger    judge.Danger  `json:"danger,omitempty"`
	Control   *ControlFacts `json:"control,omitempty"`
}

// ControlFacts are a harness snapshot, never inferred from model-written memory.
type ControlFacts struct {
	RequestID     string `json:"request_id"`
	ExecutorOwner string `json:"executor_owner,omitempty"`
	ActionOwner   string `json:"action_owner,omitempty"`
	Admission     string `json:"admission"` // ordinary or recovery
	Eligible      bool   `json:"eligible"`
	AutoPaused    bool   `json:"autonomy_paused"`
	PauseReason   string `json:"pause_reason"`
	Attempts      int    `json:"recovery_attempts"`
	Limit         int    `json:"recovery_limit"`
	Closed        bool   `json:"recovery_closed"`
	ClosureReason string `json:"recovery_closure_reason"`
}

// Options configures one run. Provider, Model, Body, Observer, and Player
// are required.
type Options struct {
	Provider harness.Provider
	Model    string
	Effort   string
	System   string // system prompt, prompts/executor.md
	Context  ExecutionContext

	Body      *body.Client
	SkillsDir string
	Observer  goal.Observer // answers the until predicate
	Player    string        // the bot's username, for the predicate

	MaxSteps      int           // default 40
	MaxDuration   time.Duration // default 5m
	CheckInterval time.Duration // how often to check until between tool calls; default 1s

	// Judge runs beside the loop. Danger ends the run; stuck stops only its
	// observed action, feeding evidence into the same conversation and budget.
	Judge    *judge.Judge
	JudgeLog io.Writer // JSONL tick log; default discard

	Log io.Writer // human-readable transcript; default discard
}

// Tokens is token usage summed over the run.
type Tokens struct {
	Input       int `json:"input"`
	CachedInput int `json:"cached_input"`
	CacheWrite  int `json:"cache_write"`
	Output      int `json:"output"`
	Total       int `json:"total"`
}

// Outcome is the result of one run, printed as JSON by `clyde run`.
type Outcome struct {
	Outcome    string           `json:"outcome"`
	Detail     string           `json:"detail,omitempty"`
	Goal       string           `json:"goal"`
	Until      string           `json:"until,omitempty"` // last predicate check
	Interrupt  *judge.Interrupt `json:"interrupt,omitempty"`
	Final      string           `json:"final,omitempty"` // the model's last text
	Model      string           `json:"model"`
	Steps      int              `json:"steps"`
	Tokens     Tokens           `json:"tokens"`
	CostUSD    float64          `json:"cost_usd"`
	Seconds    float64          `json:"seconds"`
	SkillCalls map[string]int   `json:"skill_calls,omitempty"` // actual attempted run_skill calls, including failures
}

var (
	errGoalMet   = errors.New("goal met")
	errBudget    = errors.New("wall-time budget exhausted")
	errRunEnded  = errors.New("run ended")
	stopTimeout  = 3 * time.Second
	finalChecks  = 3
	finalBackoff = 500 * time.Millisecond
)

type interruptError struct{ it judge.Interrupt }

func (e *interruptError) Error() string { return "judge interrupt: " + e.it.Reason }

// Run pursues g once. It returns an error only when the run cannot start
// (bad options, or a predicate the observer cannot check); everything after
// that is reported in the Outcome.
func Run(ctx context.Context, g goal.Goal, o Options) (Outcome, error) {
	start := time.Now()
	out := Outcome{Goal: g.Description, Model: o.Model}
	if o.Provider == nil || o.Body == nil || o.Observer == nil || o.Player == "" {
		return out, errors.New("executor: provider, body, observer, and player are required")
	}
	if o.MaxSteps <= 0 {
		o.MaxSteps = 40
	}
	if o.MaxDuration <= 0 {
		o.MaxDuration = 5 * time.Minute
	}
	if o.CheckInterval <= 0 {
		o.CheckInterval = time.Second
	}
	logw := o.Log
	if logw == nil {
		logw = io.Discard
	}
	until, err := json.Marshal(g.Until)
	if err != nil {
		return out, err
	}

	// Observers such as RCON are not safe for concurrent use.
	var checkMu sync.Mutex
	check := func(ctx context.Context) (bool, error) {
		checkMu.Lock()
		defer checkMu.Unlock()
		res, err := goal.Evaluate(ctx, o.Observer, o.Player, g.Until)
		if err == nil {
			out.Until = res.Detail
		}
		return res.Satisfied, err
	}
	if ok, err := check(ctx); err != nil {
		return out, fmt.Errorf("executor: checking until: %w", err)
	} else if ok {
		out.Outcome, out.Detail = Succeeded, "already held at start"
		return out, nil
	}

	tb := &Toolbox{Body: o.Body, SkillsDir: o.SkillsDir}
	first, err := tb.observe(ctx, nil)
	if err != nil {
		return out, fmt.Errorf("executor: first observation: %w", err)
	}

	cctx, cancel := context.WithCancelCause(ctx)
	defer cancel(errRunEnded)
	runCtx, cancelTimeout := context.WithTimeoutCause(cctx, o.MaxDuration, errBudget)
	defer cancelTimeout()

	checkNow := func() {
		if ok, err := check(runCtx); err != nil {
			if runCtx.Err() == nil {
				fmt.Fprintf(logw, "until check failed: %v\n", err)
			}
		} else if ok {
			cancel(errGoalMet)
		}
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		t := time.NewTicker(o.CheckInterval)
		defer t.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-t.C:
				checkNow()
			}
		}
	})
	if o.Judge != nil {
		if o.Context.Mode != "" {
			o.Judge.Recovering(g, o.Context.Danger)
		}
		jlog := o.JudgeLog
		if jlog == nil {
			jlog = io.Discard
		}
		wg.Go(func() {
			err := o.Judge.Run(runCtx, jlog, func(it judge.Interrupt) {
				if it.Reason == "danger" {
					fmt.Fprintf(logw, "judge: %s (%s %s)\n", it.Reason, it.Source, it.Detail)
					cancel(&interruptError{it})
				} else if it.Reason == "stuck" {
					if err := tb.stuck(runCtx, it); err != nil && runCtx.Err() == nil {
						cancel(fmt.Errorf("conditional stuck stop failed: %w", err))
					}
				}
			})
			if err != nil {
				fmt.Fprintf(logw, "judge stopped: %v\n", err)
			}
		})
	}

	user := fmt.Sprintf("Goal: %s\n\nDone when (checked automatically; the run ends as soon as it holds): %s\n\nCurrent state:\n%s",
		g.Description, until, first)
	if o.Context.Mode != "" || o.Context.Control != nil {
		intent, _ := json.Marshal(o.Context) // harness-owned finite check values
		user += "\n\nHarness execution context: " + string(intent)
	}
	var usageMu sync.Mutex
	logEvent := transcript(logw)
	var res *harness.Result
	var runErr error
	messages := []harness.Message{{Role: harness.RoleUser, Content: user}}
	completed := 0
	for {
		part, err := harness.Run(runCtx, &exploringProvider{Provider: o.Provider, tb: tb},
			harness.WithSystem(o.System),
			harness.WithMessages(messages...),
			harness.WithTools(Harness(tb.Tools())...),
			harness.WithModel(o.Model),
			harness.WithReasoning(harness.ReasoningOptions{Effort: o.Effort}),
			harness.WithMaxSteps(o.MaxSteps-completed),
			harness.WithAfterTool(func(context.Context, harness.ToolCall, *harness.ToolResult) error {
				checkNow()
				return nil
			}),
			harness.WithEventHandler(func(e harness.Event) {
				usageMu.Lock()
				defer usageMu.Unlock()
				e.Step += completed
				logEvent(e)
				if e.Type == harness.EventToolStart && e.ToolCall.Name == "run_skill" {
					var a struct {
						Name string `json:"name"`
					}
					if json.Unmarshal(e.ToolCall.Arguments, &a) == nil {
						if out.SkillCalls == nil {
							out.SkillCalls = map[string]int{}
						}
						out.SkillCalls[a.Name]++
					}
				}
			}),
		)
		runErr = err
		if part != nil {
			messages = append(messages, part.Messages...)
			if res != nil {
				part.Messages = append(res.Messages, part.Messages...)
				part.TotalUsage.Add(&res.TotalUsage)
				part.CallUsage = append(res.CallUsage, part.CallUsage...)
				part.Steps += res.Steps
			}
			res = part
			completed = res.Steps
		}
		// The provider's version check cannot cover message/event emission inside
		// harness.Run. Linearize terminal acceptance with conditional stop here.
		// Cancelling while locked also rejects a judge callback queued behind us.
		tb.controlMu.Lock()
		if runErr == nil && runCtx.Err() == nil && res != nil && tb.planned != tb.version &&
			(res.StopReason == harness.StopEndTurn || res.StopReason == harness.StopRefusal || res.StopReason == harness.StopIncomplete) {
			// Terminal refusals/incomplete replies may retain unexecuted calls.
			// Balance them without altering provider-native assistant content.
			for _, call := range res.Messages[len(res.Messages)-1].ToolCalls {
				withheld := harness.Message{Role: harness.RoleTool, ToolResult: &harness.ToolResult{
					ToolCallID: call.ID, IsError: true,
					Content: "Tool call withheld after action-local interruption; not executed.",
				}}
				messages = append(messages, withheld)
				res.Messages = append(res.Messages, withheld)
			}
			if completed < o.MaxSteps {
				tb.controlMu.Unlock()
				continue // same history, cumulative usage/steps, original deadline
			}
			res.StopReason = harness.StopMaxSteps
		}
		cancel(errRunEnded) // first cause wins; this only stops the watchers
		tb.controlMu.Unlock()
		break
	}
	wg.Wait()
	cause := context.Cause(runCtx)

	if res != nil {
		out.Steps = res.Steps
		u := res.TotalUsage
		out.Tokens = Tokens{u.InputTokens, u.CachedInputTokens, u.CacheCreationInputTokens, u.OutputTokens,
			u.InputTokens + u.CachedInputTokens + u.CacheCreationInputTokens + u.OutputTokens}
		out.CostUSD, _ = llm.Cost(o.Model, res.CallUsage)
		out.Final = finalText(res.Messages)
	}

	// Nothing keeps acting after a run: cancelling a request does not stop
	// a Minecraft action.
	stopCtx, stopCancel := context.WithTimeout(context.WithoutCancel(ctx), stopTimeout)
	defer stopCancel()
	if err := o.Body.Stop(stopCtx); err != nil {
		fmt.Fprintf(logw, "stop failed: %v\n", err)
	}
	// Did the goal hold anyway? Pickups and the last check can lag the model.
	heldAtEnd := func() bool {
		for i := range finalChecks {
			if ok, err := check(stopCtx); err == nil && ok {
				return true
			}
			if i < finalChecks-1 {
				time.Sleep(finalBackoff)
			}
		}
		return false
	}

	var ie *interruptError
	switch {
	case errors.Is(cause, errGoalMet):
		out.Outcome = Succeeded
	case errors.As(cause, &ie):
		out.Outcome, out.Interrupt = Interrupted, &ie.it
		out.Detail = fmt.Sprintf("judge: %s (%s) %s", ie.it.Reason, ie.it.Source, ie.it.Detail)
	case errors.Is(cause, errBudget):
		out.Outcome, out.Detail = BudgetExhausted, fmt.Sprintf("max time %s", o.MaxDuration)
	case cause != errRunEnded:
		out.Outcome, out.Detail = Cancelled, cause.Error()
		if heldAtEnd() {
			out.Outcome, out.Detail = Succeeded, "held when cancelled"
		}
	case runErr != nil:
		out.Outcome, out.Detail = Failed, runErr.Error()
	case res.StopReason == harness.StopMaxSteps:
		out.Outcome, out.Detail = BudgetExhausted, fmt.Sprintf("max steps %d", o.MaxSteps)
	case res.StopReason == harness.StopEndTurn:
		out.Outcome, out.Detail = Failed, "model ended its turn before the goal held"
		if heldAtEnd() {
			out.Outcome, out.Detail = Succeeded, ""
		}
	default:
		out.Outcome, out.Detail = Failed, fmt.Sprintf("provider stopped: %s %s", res.FinishReason, res.FinishDetails)
	}
	out.Seconds = time.Since(start).Seconds()
	return out, nil
}

// The provider still makes exactly one call per harness turn. A stop arriving
// during inference invalidates that response (including a premature end-turn),
// not the conversation. Withheld calls and the next attempt consume the same
// original turn/time allowance. Dispatch also checks this version atomically.
type exploringProvider struct {
	harness.Provider
	tb *Toolbox
}

func (p *exploringProvider) Chat(ctx context.Context, params harness.ChatParams) (*harness.ChatResult, error) {
	p.tb.controlMu.Lock()
	version, stall := p.tb.version, p.tb.stall
	p.tb.planned = version
	p.tb.controlMu.Unlock()
	if stall != "" {
		state, err := p.tb.Body.StateText(ctx)
		if err != nil {
			state = "fresh state unavailable: " + err.Error()
		}
		params.Messages = append(params.Messages, harness.Message{Role: harness.RoleUser,
			Content: "Harness notice (most recent action-local stop, not a new allowance): " + stall + "\nCurrent state:\n" + state})
	}
	res, err := p.Provider.Chat(ctx, params)
	p.tb.controlMu.Lock()
	changed := version != p.tb.version
	p.tb.controlMu.Unlock()
	if err == nil && res != nil && changed {
		copy := *res // Preserve returned usage, never retry inside the provider.
		copy.Message = harness.Message{Role: harness.RoleAssistant, Content: "[Harness withheld a stale response after an action-local stop; fresh evidence follows.]"}
		copy.FinishReason = harness.FinishReasonContinuation
		return &copy, nil
	}
	return res, err
}

func finalText(msgs []harness.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == harness.RoleAssistant && msgs[i].Content != "" {
			return msgs[i].Content
		}
	}
	return ""
}

// transcript logs tool calls, tool results, and model text.
func transcript(w io.Writer) func(harness.Event) {
	return func(e harness.Event) {
		switch e.Type {
		case harness.EventToolStart:
			fmt.Fprintf(w, "[%d] → %s %s\n", e.Step, e.ToolCall.Name, e.ToolCall.Arguments)
		case harness.EventToolEnd:
			if e.Result != nil {
				fmt.Fprintf(w, "[%d] ← %s%s\n", e.Step, errMark(e.Result.IsError), clip(e.Result.Content, 600))
			}
		case harness.EventMessage:
			if e.Message.Role == harness.RoleAssistant && e.Message.Content != "" {
				fmt.Fprintf(w, "[%d] model: %s\n", e.Step, e.Message.Content)
			}
		}
	}
}

func errMark(isErr bool) string {
	if isErr {
		return "ERROR "
	}
	return ""
}

func clip(s string, n int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ⏎ ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
