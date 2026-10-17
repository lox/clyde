package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	harness "github.com/lox/agent-harness"

	"github.com/lox/clyde/harness/body"
	"github.com/lox/clyde/harness/skills"
)

// Tool is defined once and adapted for both the executor loop (Harness) and
// the MCP server (harness/mcp).
type Tool struct {
	Name        string
	Description string
	Schema      json.RawMessage // JSON Schema object for the arguments
	// Run returns text for the model. An error is shown to the model as a
	// tool error, except a cancelled context, which ends the run.
	Run func(ctx context.Context, args json.RawMessage) (string, error)
}

// IsCancel reports whether a tool error means the caller's context ended,
// as opposed to a mistake the model can correct. Once the context is done,
// any failure is down to that; the error itself may carry only the cause.
func IsCancel(ctx context.Context, err error) bool {
	return err != nil && ctx.Err() != nil
}

// Harness adapts tools for agent-harness. Bad arguments and body errors come
// back to the model as tool errors and never end the loop.
func Harness(tools []Tool) []harness.Tool {
	out := make([]harness.Tool, 0, len(tools))
	for _, t := range tools {
		out = append(out, harness.Tool{
			ToolDef: harness.ToolDef{Name: t.Name, Description: t.Description, Parameters: t.Schema},
			Execute: func(ctx context.Context, call harness.ToolCall) (*harness.ToolResult, error) {
				text, err := t.Run(ctx, call.Arguments)
				if err != nil {
					if IsCancel(ctx, err) {
						return nil, ctx.Err() // agent-harness ends the run as cancelled
					}
					return &harness.ToolResult{Content: "error: " + err.Error(), IsError: true}, nil
				}
				return &harness.ToolResult{Content: text}, nil
			},
		})
	}
	return out
}

// Toolbox holds the body tools for one goal run or one MCP session. Its
// only state is the chat cursor for observe.
type Toolbox struct {
	Body      *body.Client
	SkillsDir string // admitted library; empty disables both skill tools

	mu        sync.Mutex
	chatSince int
}

// pollInterval is how often act and observe re-read the card while waiting.
const pollInterval = 250 * time.Millisecond

// Tools returns body tools and, when configured, admitted skill tools.
func (tb *Toolbox) Tools() []Tool {
	tools := []Tool{
		{
			Name: "observe",
			Description: "Read Clyde's state card: position, health, food, the running and last action, inventory, " +
				"nearby players, mobs, dropped items and visible blocks, recent events, and chat since the last observe. " +
				"Pass wait to block until the running action finishes (at most that many seconds).",
			Schema: schema(`{"wait": {"type": "number", "minimum": 0, "maximum": 30, "description": "Seconds to wait for the running action to finish first. Default 0."}}`),
			Run:    tb.observe,
		},
		{
			Name:        "act",
			Description: actDescription,
			Schema:      actSchema,
			Run:         tb.act,
		},
		{
			Name: "eval_js",
			Description: "Run JavaScript on the bot as the body of an async function and return its value and logs. " +
				"It replaces the running action. In scope: bot (mineflayer), mc (minecraft-data registry), Vec3, goals, " +
				"Movements, goto(goal), sleep(ms), log(...), signal. Use it for anything act cannot do, such as crafting, " +
				"containers, equipping, or eating.",
			Schema: schema(`{"code": {"type": "string", "description": "Body of an async function; return a JSON-serializable value."}, "timeoutMs": {"type": "integer", "minimum": 1, "maximum": 600000, "description": "Default 60000."}}`, "code"),
			Run:    tb.evalJS,
		},
		{
			Name:        "recipes_for",
			Description: "Crafting recipes that make an item, from minecraft-data: ingredients per craft, how many it makes, and whether it needs a crafting table. Does not interrupt the running action.",
			Schema:      schema(`{"item": {"type": "string", "description": "Item name such as oak_planks or stone_pickaxe."}}`, "item"),
			Run:         tb.recipesFor,
		},
		{
			Name:        "lookup",
			Description: "Facts about a block, item, food, or entity from minecraft-data: hardness, tools that harvest it, drops, stack size, food points. Unknown names return close matches. Does not interrupt the running action.",
			Schema:      schema(`{"name": {"type": "string", "description": "Block, item, or entity name such as oak_log, iron_ore, or zombie."}}`, "name"),
			Run:         tb.lookup,
		},
	}
	if tb.SkillsDir != "" {
		tools = append(tools, Tool{
			Name: "find_skill", Description: "Search admitted reusable skills. Returns full descriptions including arguments and preconditions. Run a match with run_skill.",
			Schema: schema(`{"query":{"type":"string"}}`, "query"), Run: tb.findSkill,
		}, Tool{
			Name: "run_skill", Description: "Run an admitted JavaScript skill with JSON args (see find_skill). Replaces the running action, with the same body safety, timeout, and cancellation as eval_js.",
			Schema: schema(`{"name":{"type":"string"},"args":{"type":"object","additionalProperties":true},"timeoutMs":{"type":"integer","minimum":1,"maximum":600000}}`, "name", "args"), Run: tb.runSkill,
		})
	}
	return tools
}

func schema(props string, required ...string) json.RawMessage {
	s := map[string]any{"type": "object", "properties": json.RawMessage(props)}
	if len(required) > 0 {
		s["required"] = required
	}
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return b
}

// decodeArgs decodes tool arguments, treating empty arguments as {}.
func decodeArgs(args json.RawMessage, v any) error {
	if len(args) == 0 || string(args) == "null" {
		args = json.RawMessage("{}")
	}
	if err := json.Unmarshal(args, v); err != nil {
		return fmt.Errorf("bad arguments: %w", err)
	}
	return nil
}

func (tb *Toolbox) observe(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Wait float64 `json:"wait"`
	}
	if err := decodeArgs(args, &a); err != nil {
		return "", err
	}
	if a.Wait > 0 {
		card, err := tb.Body.State(ctx)
		if err != nil {
			return "", err
		}
		if card.Action != nil {
			if _, _, err := tb.waitAction(ctx, card.Action.ID, seconds(min(a.Wait, 30))); err != nil {
				return "", err
			}
		}
	}
	text, err := tb.Body.StateText(ctx)
	if err != nil {
		return "", err
	}
	chat, err := tb.newChat(ctx)
	if err != nil {
		return "", err
	}
	if len(chat) > 0 {
		text = strings.TrimRight(text, "\n") + "\nchat since last observe:\n  " + strings.Join(chat, "\n  ") + "\n"
	}
	return text, nil
}

// maxChat bounds how many chat lines one observation carries.
const maxChat = 10

// newChat returns chat and whispers from GET /events since the last call.
func (tb *Toolbox) newChat(ctx context.Context) ([]string, error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	page, err := tb.Body.Events(ctx, tb.chatSince)
	if err != nil {
		return nil, err
	}
	tb.chatSince = page.LatestSeq
	var lines []string
	for _, e := range page.Events {
		if e.Type == "chat" || e.Type == "whisper" {
			lines = append(lines, e.Type+": "+e.Text)
		}
	}
	if len(lines) > maxChat {
		lines = lines[len(lines)-maxChat:]
	}
	return lines, nil
}

// waitAction polls the card until action id is no longer running or wait
// passes. It returns the action's latest status and whether it finished.
func (tb *Toolbox) waitAction(ctx context.Context, id int, wait time.Duration) (*body.ActionStatus, bool, error) {
	deadline := time.Now().Add(wait)
	for {
		card, err := tb.Body.State(ctx)
		if err != nil {
			return nil, false, err
		}
		switch {
		case card.Action != nil && card.Action.ID == id:
			if !time.Now().Before(deadline) {
				return card.Action, false, nil
			}
		case card.LastAction != nil && card.LastAction.ID == id:
			return card.LastAction, true, nil
		default:
			// Replaced by an action someone else started, which has also finished.
			return nil, true, nil
		}
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-time.After(min(pollInterval, time.Until(deadline)+time.Millisecond)):
		}
	}
}

func seconds(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

// actions maps each action type to its generated request type.
var actions = map[string]func() any{
	"goto":    func() any { return new(body.GotoAction) },
	"follow":  func() any { return new(body.FollowAction) },
	"collect": func() any { return new(body.CollectAction) },
	"dig":     func() any { return new(body.DigAction) },
	"place":   func() any { return new(body.PlaceAction) },
	"chat":    func() any { return new(body.ChatAction) },
	"eval":    func() any { return new(body.EvalAction) },
	"stop":    func() any { return new(body.StopAction) },
}

const (
	defaultActWait = 10.0
	maxActWait     = 60.0
)

func (tb *Toolbox) act(ctx context.Context, args json.RawMessage) (string, error) {
	var fields map[string]json.RawMessage
	if err := decodeArgs(args, &fields); err != nil {
		return "", err
	}
	wait := defaultActWait
	if w, ok := fields["wait"]; ok && string(w) != "null" {
		if err := json.Unmarshal(w, &wait); err != nil || wait < 0 {
			return "", fmt.Errorf("wait must be a non-negative number of seconds")
		}
		wait = min(wait, maxActWait)
	}
	delete(fields, "wait")
	var typ string
	_ = json.Unmarshal(fields["type"], &typ)
	newReq, ok := actions[typ]
	if !ok {
		return "", fmt.Errorf("type must be one of %s, got %s", strings.Join(actionTypes(), ", "), fields["type"])
	}
	if typ == "stop" {
		if err := tb.Body.Stop(ctx); err != nil {
			return "", err
		}
		return "stopped", nil
	}
	// The flat tool schema lets models send fields of other types; keep only
	// non-null ones, and let the generated type check the rest.
	for k, v := range fields {
		if string(v) == "null" {
			delete(fields, k)
		}
	}
	raw, _ := json.Marshal(fields)
	req := newReq()
	if err := json.Unmarshal(raw, req); err != nil {
		return "", fmt.Errorf("invalid %s action: %w", typ, err)
	}
	status, err := tb.Body.Action(ctx, req)
	if err != nil {
		return "", err
	}
	final, done, err := tb.waitAction(ctx, status.ID, seconds(wait))
	if err != nil {
		return "", err
	}
	if final == nil {
		return fmt.Sprintf("action #%d %s finished and was replaced by another action; observe to see the state", status.ID, typ), nil
	}
	if !done {
		return fmt.Sprintf("still running after %gs (it keeps running; act again to replace it, or observe with wait): %s", wait, statusJSON(final)), nil
	}
	return statusJSON(final), nil
}

// statusJSON renders an action status without the fields models do not
// need: they sent the params, and timestamps mean nothing to them.
func statusJSON(s *body.ActionStatus) string {
	var m map[string]any
	b, _ := json.Marshal(s)
	json.Unmarshal(b, &m)
	for _, k := range []string{"params", "startedAt", "endedAt", "lastMovedAt"} {
		delete(m, k)
	}
	b, _ = json.Marshal(m)
	return string(b)
}

func actionTypes() []string {
	types := make([]string, 0, len(actions))
	for t := range actions {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

func (tb *Toolbox) evalJS(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Code      string `json:"code"`
		TimeoutMs int    `json:"timeoutMs"`
	}
	if err := decodeArgs(args, &a); err != nil {
		return "", err
	}
	if strings.TrimSpace(a.Code) == "" {
		return "", fmt.Errorf("code is required")
	}
	status, err := tb.Body.Eval(ctx, a.Code, a.TimeoutMs)
	if err != nil {
		return "", err
	}
	return statusJSON(&status), nil
}

// query runs read-only code that receives arg as the constant ARG.
func (tb *Toolbox) query(ctx context.Context, code string, arg string) (string, error) {
	lit, _ := json.Marshal(arg)
	res, err := tb.Body.Query(ctx, "const ARG = "+string(lit)+"\n"+code, 10_000)
	if err != nil {
		return "", err
	}
	if res.Status != "succeeded" {
		return "", errors.New(res.Error)
	}
	return string(res.Result.Value), nil
}

func nameArg(args json.RawMessage, field string) (string, error) {
	var a map[string]any
	if err := decodeArgs(args, &a); err != nil {
		return "", err
	}
	s, _ := a[field].(string)
	s = strings.TrimPrefix(strings.TrimSpace(s), "minecraft:")
	if s == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	return s, nil
}

const recipesJS = `
const item = mc.itemsByName[ARG]
if (!item) throw new Error('unknown item ' + ARG + '; try lookup for close matches')
const name = (id) => mc.items[id]?.name ?? String(id)
const recipes = bot.recipesAll(item.id, null, true).slice(0, 20).map((r) => ({
  makes: r.result.count,
  needs: Object.fromEntries(r.delta.filter((d) => d.count < 0).map((d) => [name(d.id), -d.count])),
  craftingTable: r.requiresTable,
}))
return recipes.length ? recipes : 'no crafting recipe makes ' + ARG + ' (it may be mined, smelted, or dropped instead)'
`

func (tb *Toolbox) recipesFor(ctx context.Context, args json.RawMessage) (string, error) {
	item, err := nameArg(args, "item")
	if err != nil {
		return "", err
	}
	return tb.query(ctx, recipesJS, item)
}

const lookupJS = `
const out = {}
const itemName = (id) => mc.items[id]?.name
const b = mc.blocksByName[ARG]
if (b) {
  out.block = {
    hardness: b.hardness,
    diggable: b.diggable,
    harvestTools: b.harvestTools ? Object.keys(b.harvestTools).map(itemName) : 'any or none',
    drops: (b.drops ?? []).map((d) => itemName(typeof d === 'object' ? (d.drop?.id ?? d.drop) : d)).filter(Boolean),
  }
}
const i = mc.itemsByName[ARG]
if (i) out.item = { stackSize: i.stackSize, ...(i.maxDurability && { maxDurability: i.maxDurability }) }
const f = mc.foodsByName?.[ARG]
if (f) out.food = { foodPoints: f.foodPoints, saturation: f.saturation }
const e = mc.entitiesByName[ARG]
if (e) out.entity = { type: e.type, category: e.category }
if (Object.keys(out).length) return out
const names = new Set([...Object.keys(mc.itemsByName), ...Object.keys(mc.blocksByName), ...Object.keys(mc.entitiesByName)])
return { unknown: ARG, closeMatches: [...names].filter((n) => n.includes(ARG) || ARG.includes(n)).slice(0, 12) }
`

func (tb *Toolbox) lookup(ctx context.Context, args json.RawMessage) (string, error) {
	name, err := nameArg(args, "name")
	if err != nil {
		return "", err
	}
	return tb.query(ctx, lookupJS, name)
}

func (tb *Toolbox) findSkill(ctx context.Context, args json.RawMessage) (string, error) {
	query, err := nameArg(args, "query")
	if err != nil {
		return "", err
	}
	hits, err := (skills.Store{Dir: tb.SkillsDir}).Find(query)
	if err != nil {
		return "", err
	}
	if len(hits) == 0 {
		return "no matching skills", nil
	}
	var out []string
	for _, h := range hits {
		out = append(out, "## "+h.Name+"\n"+h.Description)
	}
	return strings.Join(out, "\n\n"), nil
}

func (tb *Toolbox) runSkill(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Name      string          `json:"name"`
		Args      json.RawMessage `json:"args"`
		TimeoutMs int             `json:"timeoutMs"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	status, err := (skills.Store{Dir: tb.SkillsDir}).Run(ctx, tb.Body, a.Name, a.Args, a.TimeoutMs)
	if err != nil {
		return "", err
	}
	if status.Status != "succeeded" {
		return "", fmt.Errorf("skill %s: %s", a.Name, statusJSON(&status))
	}
	return statusJSON(&status), nil
}
