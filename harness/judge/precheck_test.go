package judge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// quiet is a healthy, idle bot on dry land: the baseline every case mutates.
func quiet() Card {
	var c Card
	c.TS = 100_000
	c.Bot.Health, c.Bot.Food, c.Bot.Oxygen = 20, 20, 20
	return c
}

func running(c Card, id int, progress string) Card {
	c.Action = &CardAction{ID: id, Type: "goto", Status: "running", Progress: progress}
	return c
}

func checkNamed(t *testing.T, pre Prechecks, name string) Check {
	t.Helper()
	for _, c := range pre.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no %q check in %+v", name, pre.Checks)
	return Check{}
}

func TestPrecheckThresholds(t *testing.T) {
	cfg := DefaultConfig().Prechecks
	cases := []struct {
		name     string
		mutate   func(*Card)
		check    string
		fired    bool
		decision string
	}{
		{"health at threshold", func(c *Card) { c.Bot.Health = 6 }, "low_health", true, DecideDanger},
		{"health just above", func(c *Card) { c.Bot.Health = 6.5 }, "low_health", false, ""},
		{"oxygen at threshold", func(c *Card) { c.Bot.Oxygen = 6 }, "low_oxygen", true, DecideDanger},
		{"oxygen just above", func(c *Card) { c.Bot.Oxygen = 7 }, "low_oxygen", false, DecideIdle},
		{"in lava", func(c *Card) { c.Bot.InLava = true }, "in_lava", true, DecideDanger},
		{"hostile at range", func(c *Card) { c.Entities.Hostiles = []CardEntity{{"skeleton", 12}, {"zombie", 4}} }, "hostile_near", true, DecideDanger},
		{"hostile just outside", func(c *Card) { c.Entities.Hostiles = []CardEntity{{"zombie", 4.1}} }, "hostile_near", false, ""},
		{"healthy at threshold is idle", func(c *Card) { c.Bot.Health = 14 }, "idle", true, DecideIdle},
		{"just below healthy is not idle", func(c *Card) { c.Bot.Health = 13.5 }, "idle", false, ""},
		{"quiet bot is idle", func(c *Card) {}, "idle", true, DecideIdle},
		{"in water is not idle", func(c *Card) { c.Bot.InWater = true }, "idle", false, ""},
		{"running action is not idle", func(c *Card) { *c = running(*c, 1, "") }, "idle", false, ""},
		{"fresh chat is not idle", func(c *Card) { c.Events = []CardEvent{{AgeMs: 500, Type: "chat", Text: "<Bonnie> hi"}} }, "idle", false, ""},
		{"fresh damage is not idle", func(c *Card) { c.Events = []CardEvent{{AgeMs: 500, Type: "damage"}} }, "idle", false, ""},
		{"other events stay idle", func(c *Card) { c.Events = []CardEvent{{AgeMs: 500, Type: "action.succeeded"}} }, "idle", true, DecideIdle},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := quiet()
			tc.mutate(&c)
			pre := NewPrechecker(cfg).Check(c)
			if got := checkNamed(t, pre, tc.check); got.Fired != tc.fired {
				t.Errorf("%s fired = %v, want %v (%+v)", tc.check, got.Fired, tc.fired, got)
			}
			if pre.Decision != tc.decision {
				t.Errorf("decision = %q, want %q", pre.Decision, tc.decision)
			}
		})
	}
}

func TestPrecheckDangerBeatsStuck(t *testing.T) {
	p := NewPrechecker(DefaultConfig().Prechecks)
	c := running(quiet(), 1, "10 blocks remaining")
	p.Check(c)
	c.TS += 20_000
	c.Bot.Health = 3
	if got := p.Check(c).Decision; got != DecideDanger {
		t.Fatalf("decision = %q, want danger", got)
	}
}

func TestPrecheckStalled(t *testing.T) {
	cfg := DefaultConfig().Prechecks // stuck after 10s, move epsilon 0.5
	at := func(c Card, ms int64) Card { c.TS = 100_000 + ms; return c }
	moved := func(dx float64) Card {
		c := at(running(quiet(), 1, "d=9"), 12_000)
		c.Bot.Position.X = dx
		return c
	}
	start := at(running(quiet(), 1, "d=9"), 0)
	cases := []struct {
		name  string
		cards []Card
		fired bool
	}{
		{"unchanged for exactly the threshold", []Card{start, at(running(quiet(), 1, "d=9"), 10_000)}, true},
		{"unchanged just under the threshold", []Card{start, at(running(quiet(), 1, "d=9"), 9_500)}, false},
		{"progress text changed", []Card{start, at(running(quiet(), 1, "d=8"), 6_000), at(running(quiet(), 1, "d=8"), 12_000)}, false},
		{"new action restarts the clock", []Card{start, at(running(quiet(), 2, "d=9"), 12_000)}, false},
		{"first sight of an old action is not stuck", []Card{start}, false},
		{"moved at epsilon", []Card{start, moved(0.5)}, false},
		{"moved under epsilon", []Card{start, moved(0.4)}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewPrechecker(cfg)
			var pre Prechecks
			for _, c := range tc.cards {
				pre = p.Check(c)
			}
			if got := checkNamed(t, pre, "stalled"); got.Fired != tc.fired {
				t.Fatalf("stalled = %+v, want fired %v", got, tc.fired)
			}
			if tc.fired && pre.Decision != DecideStuck {
				t.Fatalf("decision = %q, want stuck", pre.Decision)
			}
		})
	}
}

func TestPrecheckChatIsNewOnlyOnce(t *testing.T) {
	p := NewPrechecker(DefaultConfig().Prechecks)
	c := quiet()
	c.Events = []CardEvent{{AgeMs: 300, Type: "chat", Text: "<Bonnie> hi Clyde"}}
	if got := p.Check(c).Decision; got != "" {
		t.Fatalf("fresh chat: decision = %q, want model", got)
	}
	// Half a second later the same message is still on the card, but it is
	// older than the previous tick, so the bot is quiet again.
	c.TS += 500
	c.Events[0].AgeMs += 500
	if got := p.Check(c).Decision; got != DecideIdle {
		t.Fatalf("seen chat: decision = %q, want idle", got)
	}
	// A card from the past (a new fixture stream) resets what was seen.
	c.TS -= 10_000
	if got := p.Check(c).Decision; got != "" {
		t.Fatalf("after reset: decision = %q, want model", got)
	}
}

// Each fixture, seen on its own, gets the pre-check decision its situation
// calls for. Lava, drowning, and player chat must never take the idle skip.
func TestFixturePrechecks(t *testing.T) {
	want := map[string]string{
		"calm_day":              DecideIdle,
		"zombie_adjacent_night": DecideDanger,
		"low_health":            DecideDanger,
		"lava":                  DecideDanger,
		"drowning":              DecideDanger,
		"goto_no_progress":      "",
		"collect_progressing":   "",
		"player_help":           "",
		"creeper_approaching":   "",
		"player_chat_casual":    "",
	}
	fixtures, err := LoadFixtures("testdata/cards")
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != len(want) {
		t.Fatalf("%d fixtures, want %d", len(fixtures), len(want))
	}
	for _, f := range fixtures {
		var c Card
		if err := json.Unmarshal(f.Card, &c); err != nil {
			t.Fatal(err)
		}
		if got := NewPrechecker(DefaultConfig().Prechecks).Check(c).Decision; got != want[f.Name] {
			t.Errorf("%s: decision = %q, want %q", f.Name, got, want[f.Name])
		}
	}
}

func TestLoadConfigOverlay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "judge.yaml")
	os.WriteFile(path, []byte("deadline: 300ms\nprechecks:\n  hostile_range: 6\n  stuck_after: 4s\n"), 0o644)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Deadline != 300*time.Millisecond || cfg.Prechecks.HostileRange != 6 || cfg.Prechecks.StuckAfter != 4*time.Second {
		t.Fatalf("overlay not applied: %+v", cfg)
	}
	if cfg.Rate != 2 || cfg.Prechecks.LowHealth != 6 {
		t.Fatalf("defaults lost: %+v", cfg)
	}
}
