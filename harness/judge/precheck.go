package judge

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// Card is the subset of the body's state card that pre-checks read. The full
// card always travels as raw JSON; this struct only decodes what code needs.
// Field names follow body/src/state.ts until api/state.schema.json lands.
type Card struct {
	TS  int64 `json:"ts"`
	Bot struct {
		Health   float64 `json:"health"`
		Food     float64 `json:"food"`
		Oxygen   float64 `json:"oxygen"`
		Position Vec     `json:"position"`
		InWater  bool    `json:"inWater"`
		InLava   bool    `json:"inLava"`
	} `json:"bot"`
	Action   *CardAction `json:"action"`
	Entities struct {
		Hostiles []CardEntity `json:"hostiles"`
	} `json:"entities"`
	Events []CardEvent `json:"events"`
}

// CardAction is the body's current action, or nil when idle.
type CardAction struct {
	ID       int    `json:"id"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Progress string `json:"progress"`
}

// CardEntity is a nearby entity.
type CardEntity struct {
	Name     string  `json:"name"`
	Distance float64 `json:"distance"`
}

// CardEvent is a recent body event; AgeMs is relative to the card's TS.
type CardEvent struct {
	AgeMs int64  `json:"ageMs"`
	Type  string `json:"type"`
	Text  string `json:"text"`
}

// Vec is a block position.
type Vec struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// PrecheckConfig holds the thresholds code uses to decide without the model.
type PrecheckConfig struct {
	// LowHealth: health at or below this is danger.
	LowHealth float64 `yaml:"low_health"`
	// LowOxygen: oxygen (0–20) at or below this is danger.
	LowOxygen float64 `yaml:"low_oxygen"`
	// HostileRange: a hostile at or within this many blocks is danger.
	HostileRange float64 `yaml:"hostile_range"`
	// StuckAfter: a running action whose position and progress have not
	// changed for at least this long is stuck.
	StuckAfter time.Duration `yaml:"stuck_after"`
	// MoveEpsilon: position changes smaller than this (blocks) do not count.
	MoveEpsilon float64 `yaml:"move_epsilon"`
	// HealthyHealth: the bot counts as healthy, for the idle skip, at or above this.
	HealthyHealth float64 `yaml:"healthy_health"`
	// AttentionEvents: event types that, when new since the last tick, send a
	// quiet bot to the model instead of skipping it.
	AttentionEvents []string `yaml:"attention_events"`
}

// Check is one pre-check's outcome, logged every tick.
type Check struct {
	Name      string  `json:"name"`
	Fired     bool    `json:"fired"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Detail    string  `json:"detail,omitempty"`
}

// Pre-check decisions. An empty decision means code cannot decide and the
// model is asked.
const (
	DecideDanger = "danger"
	DecideStuck  = "stuck"
	DecideIdle   = "idle"
)

// Prechecks is the result of running every pre-check on one card.
type Prechecks struct {
	Checks   []Check `json:"checks"`
	Decision string  `json:"decision,omitempty"`
}

// Prechecker runs the code checks. It keeps the little state that stuck
// detection and "new event" detection need, so use one per card stream.
type Prechecker struct {
	cfg PrecheckConfig

	seen        bool
	lastTS      int64
	actionID    int
	lastPos     Vec
	lastProg    string
	lastChange  int64
	actionKnown bool
}

// NewPrechecker returns a pre-checker with the given thresholds.
func NewPrechecker(cfg PrecheckConfig) *Prechecker {
	return &Prechecker{cfg: cfg}
}

// Check runs every pre-check against card. Times come from card.TS (ms), so
// fixture streams are deterministic. A card older than the previous one
// resets the stream state.
func (p *Prechecker) Check(card Card) Prechecks {
	if p.seen && card.TS < p.lastTS {
		*p = Prechecker{cfg: p.cfg}
	}
	first := !p.seen
	prevTS := p.lastTS
	p.seen, p.lastTS = true, card.TS
	c := p.cfg

	var out Prechecks
	add := func(ch Check) bool {
		out.Checks = append(out.Checks, ch)
		return ch.Fired
	}

	danger := false
	danger = add(Check{Name: "low_health", Value: card.Bot.Health, Threshold: c.LowHealth, Fired: card.Bot.Health <= c.LowHealth}) || danger
	danger = add(Check{Name: "low_oxygen", Value: card.Bot.Oxygen, Threshold: c.LowOxygen, Fired: card.Bot.Oxygen <= c.LowOxygen}) || danger
	danger = add(Check{Name: "in_lava", Value: b2f(card.Bot.InLava), Threshold: 1, Fired: card.Bot.InLava}) || danger

	nearest := math.Inf(1)
	nearName := ""
	for _, h := range card.Entities.Hostiles {
		if h.Distance < nearest {
			nearest, nearName = h.Distance, h.Name
		}
	}
	hostile := Check{Name: "hostile_near", Threshold: c.HostileRange, Value: -1}
	if nearName != "" {
		hostile.Value, hostile.Detail = nearest, nearName
		hostile.Fired = nearest <= c.HostileRange
	}
	danger = add(hostile) || danger

	stalled := p.stalled(card)
	stuck := add(stalled)

	idle := Check{Name: "idle", Threshold: c.HealthyHealth, Value: card.Bot.Health}
	var busy []string
	if card.Action != nil {
		busy = append(busy, "action running")
	}
	if len(card.Entities.Hostiles) > 0 {
		busy = append(busy, "hostiles nearby")
	}
	if card.Bot.Health < c.HealthyHealth {
		busy = append(busy, "not healthy")
	}
	if card.Bot.InLava {
		busy = append(busy, "in lava")
	}
	if card.Bot.InWater {
		busy = append(busy, "in water")
	}
	for _, e := range card.Events {
		// An event is new if it happened after the previous card; on the first
		// card of a stream every event is new.
		if slices.Contains(c.AttentionEvents, e.Type) && (first || card.TS-e.AgeMs > prevTS) {
			busy = append(busy, "new "+e.Type+" event")
			break
		}
	}
	idle.Fired = len(busy) == 0
	idle.Detail = strings.Join(busy, ", ")
	add(idle)

	switch {
	case danger:
		out.Decision = DecideDanger
	case stuck:
		out.Decision = DecideStuck
	case idle.Fired:
		out.Decision = DecideIdle
	}
	return out
}

func (p *Prechecker) stalled(card Card) Check {
	ch := Check{Name: "stalled", Threshold: p.cfg.StuckAfter.Seconds()}
	a := card.Action
	if a == nil || a.Status != "running" {
		p.actionKnown = false
		ch.Detail = "no running action"
		return ch
	}
	pos := card.Bot.Position
	moved := math.Sqrt(sq(pos.X-p.lastPos.X)+sq(pos.Y-p.lastPos.Y)+sq(pos.Z-p.lastPos.Z)) >= p.cfg.MoveEpsilon
	if !p.actionKnown || a.ID != p.actionID || moved || a.Progress != p.lastProg {
		p.actionKnown, p.actionID, p.lastPos, p.lastProg, p.lastChange = true, a.ID, pos, a.Progress, card.TS
	}
	ch.Value = float64(card.TS-p.lastChange) / 1000
	ch.Fired = ch.Value >= ch.Threshold
	ch.Detail = fmt.Sprintf("%s #%d unchanged for %.1fs", a.Type, a.ID, ch.Value)
	return ch
}

func sq(f float64) float64 { return f * f }

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
