// Disposable slice-15 reproduction. Run from harness:
// go run ../gym/local-survival.go DELAY_SECONDS THREAT_Z NEW_OUTPUT_DIR
// Or: go run ../gym/local-survival.go handoff[-stop|-timeout|-place] 0 NEW_OUTPUT_DIR
// Requires the isolated Paper gym on 25565/25575 and body on 7777.
// No paid provider: the fixture injects latency at the session Provider boundary.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	agent "github.com/lox/agent-harness"
	"github.com/lox/clyde/harness/body"
	"github.com/lox/clyde/harness/goal"
	"github.com/lox/clyde/harness/llm"
	"github.com/lox/clyde/harness/rcon"
	"github.com/lox/clyde/harness/session"
)

type fixture func(context.Context, agent.ChatParams) (*agent.ChatResult, error)

func (f fixture) Chat(ctx context.Context, p agent.ChatParams) (*agent.ChatResult, error) {
	return f(ctx, p)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 4 {
		return fmt.Errorf("want delay_seconds|handoff threat_z new_output_dir")
	}
	handoff := false
	switch os.Args[1] {
	case "handoff", "handoff-stop", "handoff-timeout", "handoff-place":
		handoff = true
	}
	delay, err := strconv.Atoi(os.Args[1])
	if !handoff && (err != nil || (delay != 5 && delay != 15 && delay != 30)) {
		return fmt.Errorf("delay must be 5, 15 or 30 seconds")
	}
	z, err := strconv.Atoi(os.Args[2])
	if err != nil || (z != 0 && z != 3) {
		return fmt.Errorf("threat z must be 0 or 3")
	}
	dir := os.Args[3]
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "transcript.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	var mu sync.Mutex
	log := func(kind string, data any) {
		mu.Lock()
		defer mu.Unlock()
		if err := json.NewEncoder(f).Encode(map[string]any{"ts": time.Now().UnixMilli(), "kind": kind, "data": data}); err != nil {
			panic(err)
		}
	}
	ctx := context.Background()
	props, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), "paper/server.properties"))
	if err != nil {
		return err
	}
	password := ""
	for line := range strings.SplitSeq(string(props), "\n") {
		if strings.HasPrefix(line, "rcon.password=") {
			password = strings.TrimPrefix(line, "rcon.password=")
		}
	}
	rc, err := rcon.Dial(ctx, "127.0.0.1:25575", password)
	if err != nil {
		return err
	}
	defer rc.Close()
	command := func(cmd string) error {
		out, err := rc.Exec(cmd)
		log("rcon", map[string]any{"command": cmd, "result": out, "error": fmt.Sprint(err)})
		return err
	}
	b := &body.Client{URL: "http://127.0.0.1:7777", HTTP: &http.Client{Timeout: 5 * time.Second}}
	if err := b.Stop(ctx); err != nil {
		return err
	}
	// Frozen arena: oak collection beside an open two-block moat; the refuge
	// across it is navigable by pathfinder parkour. Adult zombie; Clyde starts
	// unequipped. Normal difficulty, night, no regeneration. Disposable use only.
	for _, cmd := range []string{
		"difficulty peaceful", "kill @e[type=!minecraft:player]",
		"forceload add -32 -16 16 16", "time set midnight",
		"gamerule advance_time false", "gamerule spawn_mobs false",
		"gamerule natural_health_regeneration false",
		"fill -20 73 -10 10 79 10 minecraft:stone",
		"fill -20 80 -10 10 95 10 minecraft:air",
		"fill -8 74 -10 -7 79 10 minecraft:air",
		"fill 0 80 -4 0 84 -4 minecraft:oak_log",
		"gamemode survival Clyde", "clear Clyde", "effect clear Clyde",
		"effect give Clyde minecraft:instant_health 1 10 true",
		"effect give Clyde minecraft:saturation 1 10 true",
		"tp Clyde 0.5 80 0.5", "spawnpoint Clyde 0 80 8",
		"difficulty normal",
	} {
		if err := command(cmd); err != nil {
			return err
		}
	}
	time.Sleep(1500 * time.Millisecond)
	if handoff {
		return probeHandoff(ctx, b, os.Args[1], command, log)
	}
	j, err := session.Open(filepath.Join(dir, "session.db"))
	if err != nil {
		return err
	}
	defer j.Close()
	if _, err := j.Initialize(ctx, 64); err != nil {
		return err
	}
	started := make(chan struct{})
	n := 0
	provider := fixture(func(ctx context.Context, p agent.ChatParams) (*agent.ChatResult, error) {
		n++
		id := n
		log("provider.request", map[string]any{"id": id, "model": p.Model, "effort": p.Reasoning.Effort, "messages": p.Messages})
		name, args := "wait", `{"seconds":60,"reason":"fixture: await observed outcomes"}`
		if id == 1 {
			name, args = "act", `{"type":"collect","block":"oak_log","count":5,"radius":16}`
		} else if id <= 3 {
			if id == 2 {
				close(started)
			}
			timer := time.NewTimer(time.Duration(delay) * time.Second)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				log("provider.cancelled", id)
				return nil, ctx.Err()
			case <-timer.C:
			}
			name, args = "act", `{"type":"goto","x":-12,"y":80,"z":0,"range":1}`
		}
		log("provider.return", id)
		return &agent.ChatResult{Message: agent.Message{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: fmt.Sprint(id), Name: name, Arguments: json.RawMessage(args)}}}, FinishReason: agent.FinishReasonToolUse, Usage: &agent.Usage{}}, nil
	})
	p := session.Player{Journal: j, Body: b, Provider: provider, Config: llm.ExecutorConfig(), CallTimeout: 45 * time.Second, PollInterval: 500 * time.Millisecond,
		Check: func(ctx context.Context, g goal.Goal) (goal.Result, error) {
			return goal.Evaluate(ctx, goal.RCON{Conn: rc}, "Clyde", g.Until)
		}}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- p.Run(runCtx) }()
	defer func() { cancel(); log("session.end", fmt.Sprint(<-done)) }()
	select {
	case <-started:
	case <-time.After(15 * time.Second):
		return fmt.Errorf("session never entered delayed inference")
	}
	// Wait one second into the delayed request, while collection is running.
	time.Sleep(time.Second)
	card, err := b.State(ctx)
	if err != nil {
		return err
	}
	log("pre-threat", card)
	if card.Action == nil || card.Action.Type != "collect" {
		return fmt.Errorf("collection was not running at threat introduction")
	}
	if err := command(fmt.Sprintf("summon minecraft:zombie 4.5 80 %d {Tags:[\"survival-threat\"],IsBaby:0b,PersistenceRequired:1b}", z)); err != nil {
		return err
	}
	log("threat", map[string]any{"delay_seconds": delay, "z": z})
	end := time.Now().Add(time.Duration(delay+12) * time.Second)
	page, err := b.Events(ctx, 0)
	if err != nil {
		return err
	}
	cursor := page.LatestSeq
	for time.Now().Before(end) {
		card, err := b.State(ctx)
		if err != nil {
			return err
		}
		log("state", card)
		page, err := b.Events(ctx, cursor)
		if err != nil {
			return err
		}
		log("events", page.Events)
		cursor = page.LatestSeq
		for _, cmd := range []string{"data get entity Clyde Pos", "data get entity Clyde Health"} {
			if err := command(cmd); err != nil {
				return err
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	s, err := j.Load(ctx)
	if err != nil {
		return err
	}
	log("session.final", s)
	return nil
}

// Keep the real pathfinder, physics, equip and dig implementations. Only delay
// equip's completion, exposing the window deterministically. The query installs
// diagnostics outside any action, just as the plugin's physics listener is.
func probeHandoff(ctx context.Context, b *body.Client, mode string, command func(string) error, log func(string, any)) error {
	setup := []string{"difficulty peaceful", "fill 2 80 -5 2 82 5 stone", "give Clyde diamond_pickaxe"}
	if mode == "handoff-place" {
		setup = []string{"difficulty peaceful", "fill 2 74 -10 4 79 10 air", "give Clyde dirt 16"}
	}
	for _, cmd := range setup {
		if err := command(cmd); err != nil {
			return err
		}
	}
	query := func(code string) (body.QueryResult, error) {
		r, err := b.Query(ctx, code, 2000)
		if err == nil && r.Status != "succeeded" {
			err = fmt.Errorf("probe query: %s", r.Error)
		}
		return r, err
	}
	_, err := query(`
const equip = bot.equip, dig = bot.dig, placeBlock = bot.placeBlock;
bot.survivalProbe = { events: [], equip, dig, placeBlock };
bot.equip = async (...args) => {
  bot.survivalProbe.events.push({kind: 'equip-start', at: Date.now()});
  await new Promise(resolve => { bot.survivalProbe.release = resolve });
  return equip.apply(bot, args);
};
bot.dig = (...args) => {
  bot.survivalProbe.events.push({kind: 'dig', at: Date.now(), position: args[0].position, goal: bot.pathfinder.goal});
  return dig.apply(bot, args);
};
bot.placeBlock = (...args) => {
  bot.survivalProbe.events.push({kind: 'place', at: Date.now(), position: args[0].position.plus(args[1]), goal: bot.pathfinder.goal});
  return placeBlock.apply(bot, args);
};
return true;`)
	if err != nil {
		return err
	}
	defer func() {
		if err := b.Stop(ctx); err != nil {
			log("cleanup.error", err.Error())
		}
		if _, err := query(`const p = bot.survivalProbe; p.release?.(); bot.equip = p.equip; bot.dig = p.dig; bot.placeBlock = p.placeBlock; return true`); err != nil {
			log("cleanup.error", err.Error())
		}
	}()
	var request any = map[string]any{"type": "goto", "x": 5, "y": 80, "z": 0, "range": 0}
	if mode == "handoff-timeout" {
		request = map[string]any{"type": "eval", "code": "await goto(new goals.GoalBlock(5,80,0))", "timeoutMs": 1500}
	}
	if mode == "handoff-place" {
		request = map[string]any{"type": "eval", "code": "const m = new Movements(bot); m.canDig = false; m.allowParkour = false; bot.pathfinder.setMovements(m); await goto(new goals.GoalBlock(5,80,0))", "timeoutMs": 10000}
	}
	first, err := b.Action(ctx, request)
	if err != nil {
		return err
	}
	log("first-action", first)
	ready := false
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); {
		r, err := query(`return !!bot.survivalProbe.release`)
		if err != nil {
			return err
		}
		if string(r.Result.Value) == "true" {
			ready = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		return fmt.Errorf("pathfinder did not enter equip; no handoff evidence")
	}
	if mode == "handoff-timeout" {
		time.Sleep(1600 * time.Millisecond)
		card, err := b.State(ctx)
		if err != nil {
			return err
		}
		log("after-timeout", card)
		if card.Action != nil || card.LastAction == nil || card.LastAction.Error == nil || !strings.Contains(*card.LastAction.Error, "timed out") {
			return fmt.Errorf("eval did not time out before releasing equip")
		}
	}
	if mode != "handoff" {
		if err := b.Stop(ctx); err != nil {
			return err
		}
		idle, err := query(`return {goal: bot.pathfinder.goal, moving: bot.pathfinder.isMoving(), mining: bot.pathfinder.isMining(), building: bot.pathfinder.isBuilding(), targetDigBlock: bot.targetDigBlock, events: bot.survivalProbe.events}`)
		if err != nil {
			return err
		}
		log("stopped-controller", idle.Result.Value)
	}
	if mode != "handoff-stop" {
		second, err := b.Action(ctx, map[string]any{"type": "goto", "x": -12, "y": 80, "z": 0, "range": 1})
		if err != nil {
			return err
		}
		log("replacement-action", second)
	}
	if _, err := query(`bot.survivalProbe.release(); return true`); err != nil {
		return err
	}
	time.Sleep(1200 * time.Millisecond)
	r, err := query(`return bot.survivalProbe.events`)
	if err != nil {
		return err
	}
	log("controller-events", r.Result.Value)
	if mode == "handoff-place" {
		return command("execute if block 2 79 0 minecraft:dirt")
	}
	return command("execute if block 2 81 0 minecraft:air")
}
