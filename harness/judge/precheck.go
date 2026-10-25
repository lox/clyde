package judge

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/lox/clyde/harness/body"
)

// PrecheckConfig holds the thresholds code uses to decide without the model.
type PrecheckConfig struct {
	// LowHealth: health at or below this is danger.
	LowHealth float64 `yaml:"low_health"`
	// LowOxygen: oxygen (0–20) at or below this is danger.
	LowOxygen float64 `yaml:"low_oxygen"`
	// HostileRange: a hostile at or within this many blocks is danger.
	HostileRange float64 `yaml:"hostile_range"`
	// StuckAfter: a running action that has made no progress for at least
	// this long is stuck. Progress is a new closest distanceRemaining for
	// goto, and movement, a progress text change, or a higher collected
	// count for everything else. goto and follow also stall when their
	// lastMovedAt is this old, except a follow within range of its player.
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

	seen   bool
	lastTS int64

	// Stall tracking resets across replacements and body incarnations.
	actionToken   string
	lastPos       body.Vec
	lastProg      string
	lastCollected int
	bestDist      float64
	lastChange    int64
}

// NewPrechecker returns a pre-checker with the given thresholds.
func NewPrechecker(cfg PrecheckConfig) *Prechecker {
	return &Prechecker{cfg: cfg}
}

// Danger is a factual snapshot of fired danger checks, not a recovery strategy.
type Danger []Check

// Dangers uses the same thresholds as the judge, without stall/idle policy.
func Dangers(card body.Card, cfg PrecheckConfig) Danger {
	return dangers(NewPrechecker(cfg).Check(card))
}

func dangers(pre Prechecks) Danger {
	var out Danger
	for _, c := range pre.Checks {
		if c.Fired && c.Name != "stalled" && c.Name != "idle" {
			out = append(out, c)
		}
	}
	return out
}

// difference reports appeared/cleared conditions, without deciding whether
// a recovery should stop. Numeric changes remain in the preceding/current checks.
func (d Danger) difference(previous Danger) Danger {
	var changed Danger
	for _, c := range d {
		if !slices.ContainsFunc(previous, func(p Check) bool { return c.Name == p.Name && c.Detail == p.Detail }) {
			changed = append(changed, c)
		}
	}
	return changed
}

// Check runs every pre-check against card. Times come from card.Ts (ms), so
// fixture streams are deterministic. A card older than the previous one
// resets the stream state.
func (p *Prechecker) Check(card body.Card) Prechecks {
	ts := int64(card.Ts)
	if p.seen && ts < p.lastTS {
		*p = Prechecker{cfg: p.cfg}
	}
	first := !p.seen
	prevTS := p.lastTS
	p.seen, p.lastTS = true, ts
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
		if slices.Contains(c.AttentionEvents, e.Type) && (first || ts-int64(e.AgeMs) > prevTS) {
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

func (p *Prechecker) stalled(card body.Card) Check {
	ch := Check{Name: "stalled", Threshold: p.cfg.StuckAfter.Seconds()}
	a := card.Action
	if a == nil || a.Status != body.ActionStatusStatusRunning {
		p.actionToken = ""
		ch.Detail = "no running action"
		return ch
	}
	ts := int64(card.Ts)
	pos := card.Bot.Position
	prog := orZero(a.Progress)
	collected := orZero(a.Collected)
	dist := math.Inf(1)
	if a.DistanceRemaining != nil {
		dist = *a.DistanceRemaining
	}
	eps := p.cfg.MoveEpsilon

	if a.Token != p.actionToken || p.actionToken == "" {
		p.actionToken, p.lastPos, p.lastProg, p.lastCollected, p.bestDist, p.lastChange = a.Token, pos, prog, collected, dist, ts
	}
	var progressed bool
	if a.Type == body.ActionStatusTypeGoto && a.DistanceRemaining != nil {
		// Walking back and forth is not progress; getting closer is.
		progressed = dist <= p.bestDist-eps
	} else {
		moved := math.Sqrt(sq(pos.X-p.lastPos.X)+sq(pos.Y-p.lastPos.Y)+sq(pos.Z-p.lastPos.Z)) >= eps
		progressed = moved || prog != p.lastProg || collected > p.lastCollected
	}
	if progressed {
		p.lastPos, p.lastProg, p.lastCollected, p.bestDist, p.lastChange = pos, prog, collected, min(dist, p.bestDist), ts
	}
	ch.Value = float64(ts-p.lastChange) / 1000
	ch.Detail = fmt.Sprintf("%s #%d no progress for %.1fs", a.Type, a.ID, ch.Value)
	// The body tracks movement itself, so one card can show a stall.
	if a.LastMovedAt != nil {
		if still := float64(ts-int64(*a.LastMovedAt)) / 1000; still > ch.Value {
			ch.Value = still
			ch.Detail = fmt.Sprintf("%s #%d has not moved for %.1fs", a.Type, a.ID, still)
		}
	}
	// A follow that is keeping up with a player who stands still is fine.
	if a.Type == body.ActionStatusTypeFollow && dist <= followRange(a)+1 {
		p.lastChange = ts
		ch.Value, ch.Detail = 0, fmt.Sprintf("follow #%d within range", a.ID)
	}
	ch.Fired = ch.Value >= ch.Threshold
	return ch
}

func followRange(a *body.ActionStatus) float64 {
	if r, ok := a.Params["range"].(float64); ok {
		return r
	}
	return 3 // the body's default
}

func orZero[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func sq(f float64) float64 { return f * f }

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
