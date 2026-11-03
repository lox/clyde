// Model-free slice-15 conformance probe. Disposable local Paper/body only.
// From harness: go run ../gym/cancellation.go MODE ENDING NEW_OUTPUT_DIR
// Restart survival-body between cases; runtime instrumentation is not a fix.
package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lox/clyde/harness/body"
	"github.com/lox/clyde/harness/rcon"
)

//go:embed cancellation-fixture.js
var instrumentation string

func main() {
	if err := probe(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func probe() error {
	if len(os.Args) != 4 {
		return fmt.Errorf("want mode ending new_output_dir; see docs/plans/15-local-survival.md")
	}
	mode, ending := os.Args[1], os.Args[2]
	phases := map[string]string{
		"dig-look": "look", "place-look": "look", "equip": "equip", "active-dig": "active-dig",
		"collector-profile": "done", "collector-drop": "collector-drop", "collector-pickup": "collector-pickup", "collector-error": "collector-error",
		"local-navigation": "navigation", "local-look": "look", "local-dig": "active-dig", "local-drop": "drop", "local-equip": "equip", "local-stuck": "look",
		"path-dig": "path-equip", "path-place": "path-equip", "path-dig-movement": "done", "path-place-movement": "done",
	}
	phase, ok := phases[mode]
	if !ok {
		return fmt.Errorf("unknown mode %q", mode)
	}
	switch ending {
	case "stop", "replace", "timeout", "disconnect", "local", "danger", "complete":
	default:
		return fmt.Errorf("unknown ending %q", ending)
	}
	if err := os.MkdirAll(os.Args[3], 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(os.Args[3], "transcript.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	log := func(kind string, data any) {
		if err := json.NewEncoder(f).Encode(map[string]any{"ts": time.Now().UnixMilli(), "kind": kind, "data": data}); err != nil {
			panic(err)
		}
	}
	log("fixture", map[string]any{"mode": mode, "ending": ending, "sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(instrumentation)))})
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
	command := func(cmd string) (string, error) {
		out, err := rc.Exec(cmd)
		log("rcon", map[string]any{"command": cmd, "result": out, "error": fmt.Sprint(err)})
		return out, err
	}
	b := &body.Client{URL: "http://127.0.0.1:7777", HTTP: &http.Client{Timeout: 5 * time.Second}}
	query := func(code string) (json.RawMessage, error) {
		r, err := b.Query(ctx, code, 2000)
		if err == nil && r.Status != "succeeded" {
			err = fmt.Errorf("query: %s", r.Error)
		}
		return r.Result.Value, err
	}
	if err := b.Stop(ctx); err != nil {
		return err
	}
	// Same arena geometry as the six original trials, without a hostile for
	// phase-controlled conformance. The danger control adds one explicitly.
	commands := []string{
		"difficulty peaceful", "kill @e[type=!minecraft:player]", "forceload add -32 -16 16 16",
		"time set midnight", "gamerule advance_time false", "gamerule spawn_mobs false", "gamerule natural_health_regeneration false",
		"fill -20 73 -10 10 79 10 stone", "fill -20 80 -10 10 95 10 air", "fill -8 74 -10 -7 79 10 air",
		"fill 0 80 -4 0 84 -4 oak_log", "gamemode survival Clyde", "clear Clyde", "effect clear Clyde",
		"effect give Clyde instant_health 1 10 true", "effect give Clyde saturation 1 10 true",
		"tp Clyde 0.5 80 0.5", "spawnpoint Clyde 0 80 8", "difficulty normal",
	}
	if mode == "equip" || mode == "local-equip" {
		commands = append(commands, "item replace entity Clyde inventory.0 with diamond_axe")
	}
	if mode == "place-look" {
		commands = append(commands, "give Clyde dirt 16")
	}
	if mode == "collector-pickup" {
		commands = append(commands, `summon item 0.5 80 0.5 {Item:{id:"minecraft:oak_log",count:1},PickupDelay:32767s}`)
	}
	if strings.HasPrefix(mode, "path-dig") {
		commands = append(commands, "fill 2 80 -5 2 82 5 stone", "give Clyde diamond_pickaxe")
	}
	if strings.HasPrefix(mode, "path-place") {
		commands = append(commands, "fill 2 74 -10 4 79 10 air", "give Clyde dirt 16")
	}
	for _, cmd := range commands {
		if _, err := command(cmd); err != nil {
			return err
		}
	}
	time.Sleep(750 * time.Millisecond)
	if _, err := query(instrumentation); err != nil {
		return err
	}
	timeout := 20000
	if ending == "timeout" {
		timeout = 1500
	}
	request := map[string]any{"type": "eval", "code": fmt.Sprintf("await bot.cancellationProbe.run(%q, signal, goto)", mode), "timeoutMs": timeout}
	if mode == "collector-drop" || mode == "collector-error" || mode == "collector-profile" {
		if _, err := query(fmt.Sprintf("await bot.cancellationProbe.run(%q, signal, goto, true); return true", mode)); err != nil {
			return err
		}
		request = map[string]any{"type": "collect", "block": "oak_log", "count": 1, "radius": 16}
	}
	first, err := b.Action(ctx, request)
	if err != nil {
		return err
	}
	log("first-action", first)
	ready := false
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); {
		value, err := query(`return {phase: bot.cancellationProbe.phase, active: !!bot.targetDigBlock,
  settled: bot.cancellationProbe.events.some(e=>e.kind==='collect-settled')}`)
		if err != nil {
			return err
		}
		var state struct {
			Phase   string `json:"phase"`
			Active  bool   `json:"active"`
			Settled bool   `json:"settled"`
		}
		if err := json.Unmarshal(value, &state); err != nil {
			return err
		}
		if state.Phase == phase && (phase != "active-dig" || state.Active) || mode == "collector-profile" && state.Settled {
			ready = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		state, _ := b.State(ctx)
		log("fixture-error", state)
		return fmt.Errorf("did not reach phase %s; inconclusive, not conformance pass", phase)
	}
	if _, err := query(`bot.cancellationProbe.mark('boundary'); return bot.cancellationProbe.events`); err != nil {
		return err
	}
	if ending != "local" && ending != "danger" && ending != "complete" {
		if _, err := query(`const p=bot.cancellationProbe; p.revoked=Date.now(); p.mark('revoked'); return true`); err != nil {
			return err
		}
	}
	if ending == "timeout" {
		time.Sleep(1600 * time.Millisecond)
		state, err := b.State(ctx)
		if err != nil {
			return err
		}
		log("timeout-state", state)
		if state.Action != nil || state.LastAction == nil || state.LastAction.Error == nil || !strings.Contains(*state.LastAction.Error, "timed out") {
			return fmt.Errorf("action did not time out at held boundary")
		}
	}
	if ending == "local" || ending == "danger" {
		if _, err := query(`bot.cancellationProbe.trigger(); return true`); err != nil {
			return err
		}
	}
	if ending == "danger" {
		if _, err := command(`summon zombie 4.5 80 0 {IsBaby:0b,PersistenceRequired:1b}`); err != nil {
			return err
		}
		// Exercise the same incarnation-conditional danger stop used by session.
		// No provider is involved and no production danger check is disabled.
		stopped, err := b.StopAction(ctx, first.Token)
		if err != nil {
			return err
		}
		log("danger-stop", stopped)
		if stopped == nil {
			return fmt.Errorf("danger stop did not match the foreground owner")
		}
	}
	if ending == "stop" {
		if err := b.Stop(ctx); err != nil {
			return err
		}
	}
	if ending == "replace" {
		r, err := b.Action(ctx, map[string]any{"type": "eval", "code": `bot.collectBlock.targets.appendTarget(bot.blockAt(new Vec3(0,84,-4))); await bot.cancellationProbe.retreat(signal, goto)`, "timeoutMs": 10000})
		if err != nil {
			return err
		}
		log("replacement", r)
	}
	boundary, err := query(`const p = bot.cancellationProbe; return {events:p.events, goal:bot.pathfinder.goal}`)
	if err != nil {
		return err
	}
	log("boundary", boundary)
	if ending == "disconnect" {
		// Release after the server has kicked the player. Production body exits
		// on end; the scheduled continuation must not send to a new connection.
		if _, err := query(`setTimeout(() => bot.cancellationProbe.release?.(), 800); return true`); err != nil {
			return err
		}
		if _, err := command("kick Clyde conformance_disconnect"); err != nil {
			return err
		}
		time.Sleep(5 * time.Second)
		state, stateErr := b.State(ctx)
		if stateErr == nil {
			// The orb supervisor may already have restarted the process. A new
			// empty body is correct; a live old fixture or inherited owner is not.
			v, err := query(`return bot.cancellationProbe === undefined`)
			if err != nil {
				return err
			}
			log("restart-state", state)
			if string(v) != "true" || state.Action != nil || state.LastAction != nil {
				return fmt.Errorf("old fixture or owner survived disconnect")
			}
		}
		log("assert", map[string]any{"name": "disconnect destroys old owner and continuation", "pass": true})
		for _, cmd := range []string{"execute if block 0 80 -4 oak_log", "execute if block 1 80 0 air"} {
			out, err := command(cmd)
			if err != nil {
				return err
			}
			if !strings.Contains(out, "Test passed") {
				return fmt.Errorf("server world changed after disconnect: %s", out)
			}
		}
		return nil
	}
	if mode == "local-stuck" {
		time.Sleep(700 * time.Millisecond)
		v, err := query(`return {handoff: !!bot.cancellationProbe.retreatAt, events:bot.cancellationProbe.events}`)
		if err != nil {
			return err
		}
		log("held-past-deadline", v)
	}
	if _, err := query(fmt.Sprintf(`const p=bot.cancellationProbe; p.release?.(%t); p.resumeTicks?.(); return true`, mode == "collector-error")); err != nil {
		return err
	}
	if mode == "collector-pickup" {
		if _, err := command("kill @e[type=item]"); err != nil {
			return err
		}
	}
	// Allow late cleanup and the original unarmed dig timer to fire. Movement
	// is verified independently below; a fixture's return is not evidence.
	time.Sleep(5500 * time.Millisecond)
	value, err := query(`const p=bot.cancellationProbe; return {events:p.events, phase:p.phase, goal:bot.pathfinder.goal,
  trigger:p.revoked, handoff:p.retreatAt, commanded:p.commandedAt, observed:p.observedAt,
  targets:bot.collectBlock.targets.targets.map(t=>t.position),
  profile:{canDig:bot.pathfinder.movements.canDig, scaffolding:bot.pathfinder.movements.scafoldingBlocks, doors:bot.pathfinder.movements.canOpenDoors, parkour:bot.pathfinder.movements.allowParkour},
  packets:p.events.filter(e=>e.kind==='packet' && !(e.name==='block_dig' && e.data.status===1))}`)
	if err != nil {
		return err
	}
	log("probe", value)
	state, err := b.State(ctx)
	if err != nil {
		return err
	}
	log("state", state)
	page, err := b.Events(ctx, 0)
	if err != nil {
		return err
	}
	log("body-events", page)
	for _, cmd := range []string{"data get entity Clyde Pos", "data get entity Clyde Health", "data get entity Clyde Inventory", "execute if block 0 80 -4 oak_log", "execute if block 1 80 0 air"} {
		if _, err := command(cmd); err != nil {
			return err
		}
	}
	var result struct {
		Packets []struct {
			Name string
			At   int64
		} `json:"packets"`
		Events []struct {
			Kind string
			At   int64
		} `json:"events"`
		Targets   []struct{ X, Y, Z int } `json:"targets"`
		Trigger   int64                   `json:"trigger"`
		Handoff   int64                   `json:"handoff"`
		Commanded int64                   `json:"commanded"`
	}
	if err := json.Unmarshal(value, &result); err != nil {
		return err
	}
	failed := false
	check := func(name string, pass bool) {
		log("assert", map[string]any{"name": name, "pass": pass})
		fmt.Printf("%s %s: %s = %t\n", mode, ending, name, pass)
		failed = failed || !pass
	}
	// Equip is explicitly noninterruptible only in the serialized local fixture.
	// Legacy replacement is a failure: a new owner ran before it settled.
	// HTTP request/response time is not revocation time. Use the body's terminal
	// event for this exact action, or the fixture's synchronous local trigger.
	revokedAt := result.Trigger
	if ending != "local" && ending != "danger" && ending != "complete" {
		revokedAt = 0
		for _, event := range page.Events {
			if event.Data["token"] == first.Token && (event.Type == "action.aborted" || event.Type == "action.failed") {
				revokedAt = event.TS
				break
			}
		}
		check("recorded actual action revocation", revokedAt > 0)
	}
	unauthorized := 0
	equipSettled := int64(0)
	for _, event := range result.Events {
		if event.Kind == "equip-settled" {
			equipSettled = event.At
		}
	}
	for _, packet := range result.Packets {
		if revokedAt == 0 || packet.At < revokedAt {
			continue
		}
		if mode == "local-equip" && ending == "local" && packet.Name == "window_click" && result.Handoff > 0 && packet.At <= result.Handoff {
			continue
		}
		unauthorized++
	}
	check("no unauthorized late mutation packets", unauthorized == 0)
	if mode == "local-equip" && ending == "local" {
		check("equip settled before handoff", equipSettled > 0 && equipSettled <= result.Handoff)
	}
	if ending == "replace" && strings.HasPrefix(mode, "collector-") {
		check("old cleanup preserves replacement target", len(result.Targets) == 1 && result.Targets[0].X == 0 && result.Targets[0].Y == 84 && result.Targets[0].Z == -4)
	}
	blockCheck := ""
	switch {
	case mode == "dig-look" || mode == "active-dig":
		blockCheck = "execute if block 0 80 -4 oak_log"
	case mode == "place-look":
		blockCheck = "execute if block 1 80 0 air"
	case strings.HasPrefix(mode, "path-dig"):
		blockCheck = "execute if block 2 81 0 stone"
	case strings.HasPrefix(mode, "path-place"):
		blockCheck = "execute if block 2 79 0 air"
	}
	if blockCheck != "" {
		out, err := command(blockCheck)
		if err != nil {
			return err
		}
		check("server target unchanged", strings.Contains(out, "Test passed"))
	}
	if ending == "local" {
		check("handoff <=500ms", result.Handoff >= result.Trigger && result.Handoff-result.Trigger <= 500)
		check("commanded movement <=1000ms", result.Commanded >= result.Trigger && result.Commanded-result.Trigger <= 1000)
		out, err := command(`execute positioned -12 80 0 if entity @a[name=Clyde,distance=..2]`)
		if err != nil {
			return err
		}
		check("server refuge arrival", strings.Contains(out, "Test passed"))
	}
	if strings.HasSuffix(mode, "-movement") {
		check("navigation emitted no world or inventory mutation", len(result.Packets) == 0)
		out, err := command(`execute positioned 5 80 0 if entity @a[name=Clyde,distance=..2]`)
		if err != nil {
			return err
		}
		check("server navigation arrival", strings.Contains(out, "Test passed"))
	}
	if mode == "local-stuck" && ending == "local" {
		check("expired handoff stays closed after release", result.Handoff == 0 && result.Commanded == 0)
	}
	if ending == "danger" {
		out, err := command(`execute positioned -12 80 0 if entity @a[name=Clyde,distance=..2]`)
		if err != nil {
			return err
		}
		check("response survives ordinary danger preemption", strings.Contains(out, "Test passed"))
	}
	if mode == "collector-profile" {
		out, err := command("execute if block 0 80 -4 air")
		if err != nil {
			return err
		}
		check("movement-only collector harvested target", strings.Contains(out, "Test passed"))
	}
	if err := b.Stop(ctx); err != nil {
		return err
	}
	if failed {
		return fmt.Errorf("conformance no-go: %s/%s (retained evidence, not a passing test)", mode, ending)
	}
	return nil
}
