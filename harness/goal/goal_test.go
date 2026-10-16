package goal

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fakeObserver struct {
	items      map[ItemID]int
	pos        *Vec3
	blocks     map[BlockPos]BlockID
	containers map[BlockPos]map[ItemID]int
}

func (f fakeObserver) ItemCount(_ context.Context, _ string, item ItemID) (int, error) {
	return f.items[item], nil
}

func (f fakeObserver) Position(context.Context, string) (Vec3, bool, error) {
	if f.pos == nil {
		return Vec3{}, false, nil
	}
	return *f.pos, true, nil
}

func (f fakeObserver) BlockMatches(_ context.Context, pos BlockPos, block BlockID) (bool, error) {
	return f.blocks[pos] == block, nil
}

func (f fakeObserver) ContainerCount(_ context.Context, pos BlockPos, item ItemID) (int, error) {
	c, ok := f.containers[pos]
	if !ok {
		return 0, errors.New("not a container")
	}
	return c[item], nil
}

func TestEvaluate(t *testing.T) {
	chest := BlockPos{X: -307, Y: 79, Z: 151}
	table := []BlockAt{
		{Pos: BlockPos{X: 1, Y: 64, Z: 1}, Block: "minecraft:crafting_table"},
		{Pos: BlockPos{X: 2, Y: 64, Z: 1}, Block: "minecraft:furnace"},
		{Pos: BlockPos{X: 1, Y: 65, Z: 1}, Block: "minecraft:oak_planks"},
	}
	built := map[BlockPos]BlockID{}
	for _, b := range table {
		built[b.Pos] = b.Block
	}
	oneWrong := map[BlockPos]BlockID{}
	for k, v := range built {
		oneWrong[k] = v
	}
	oneWrong[table[2].Pos] = "minecraft:birch_planks"

	items := &ItemsPredicate{Item: "minecraft:oak_log", Count: 4}
	arrived := &ArrivedPredicate{At: Vec3{X: 10.5, Y: 64, Z: -3.5}, Radius: 2.5}
	delivered := &DeliveredPredicate{Container: chest, Item: "minecraft:oak_log", Count: 8}

	tests := []struct {
		name string
		obs  fakeObserver
		p    Predicate
		want bool
	}{
		{"items one short", fakeObserver{items: map[ItemID]int{"minecraft:oak_log": 3}}, Predicate{Items: items}, false},
		{"items exactly", fakeObserver{items: map[ItemID]int{"minecraft:oak_log": 4}}, Predicate{Items: items}, true},
		{"items other item only", fakeObserver{items: map[ItemID]int{"minecraft:birch_log": 9}}, Predicate{Items: items}, false},

		// A 3-4-5 diagonal lands exactly on the radius, so a per-axis, squared, or
		// strict-inequality bug shows.
		{"arrived on radius", fakeObserver{pos: &Vec3{X: 10.5 + 1.5, Y: 64, Z: -3.5 - 2}}, Predicate{Arrived: arrived}, true},
		{"arrived just outside radius", fakeObserver{pos: &Vec3{X: 10.5 + 2.51, Y: 64, Z: -3.5}}, Predicate{Arrived: arrived}, false},
		{"arrived height counts", fakeObserver{pos: &Vec3{X: 10.5, Y: 64 + 2.51, Z: -3.5}}, Predicate{Arrived: arrived}, false},
		{"arrived offline", fakeObserver{}, Predicate{Arrived: arrived}, false},

		{"built all match", fakeObserver{blocks: built}, Predicate{Built: &BuiltPredicate{Blocks: table}}, true},
		{"built one wrong", fakeObserver{blocks: oneWrong}, Predicate{Built: &BuiltPredicate{Blocks: table}}, false},
		{"built empty", fakeObserver{}, Predicate{Built: &BuiltPredicate{}}, false},

		{"delivered one short", fakeObserver{containers: map[BlockPos]map[ItemID]int{chest: {"minecraft:oak_log": 7}}}, Predicate{Delivered: delivered}, false},
		{"delivered exactly", fakeObserver{containers: map[BlockPos]map[ItemID]int{chest: {"minecraft:oak_log": 8}}}, Predicate{Delivered: delivered}, true},
		{"delivered to wrong container", fakeObserver{containers: map[BlockPos]map[ItemID]int{
			chest:                           {},
			{X: chest.X + 1, Y: 79, Z: 151}: {"minecraft:oak_log": 64},
		}}, Predicate{Delivered: delivered}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Evaluate(context.Background(), tt.obs, "clyde", tt.p)
			if err != nil {
				t.Fatal(err)
			}
			if got.Satisfied != tt.want {
				t.Fatalf("satisfied = %v, want %v (%s)", got.Satisfied, tt.want, got.Detail)
			}
		})
	}
}

func TestEvaluateRejectsBadPredicates(t *testing.T) {
	ctx := context.Background()
	if _, err := Evaluate(ctx, fakeObserver{}, "clyde", Predicate{}); err == nil {
		t.Error("empty predicate: want error")
	}
	two := Predicate{Items: &ItemsPredicate{Item: "minecraft:dirt", Count: 1}, Arrived: &ArrivedPredicate{}}
	if _, err := Evaluate(ctx, fakeObserver{}, "clyde", two); err == nil {
		t.Error("two kinds: want error")
	}
	if _, err := Evaluate(ctx, fakeObserver{}, "clyde", Predicate{Judged: &JudgedPredicate{Question: "nice?"}}); !errors.Is(err, ErrJudged) {
		t.Errorf("judged: got %v, want ErrJudged", err)
	}
	if _, err := Evaluate(ctx, fakeObserver{}, "clyde", Predicate{Delivered: &DeliveredPredicate{Item: "minecraft:dirt", Count: 1}}); err == nil {
		t.Error("delivered to a non-container: want error")
	}
}

func TestGoalJSONValidation(t *testing.T) {
	tests := []struct {
		name, json string
		ok         bool
	}{
		{"valid", `{"description":"get logs","until":{"items":{"item":"minecraft:oak_log","count":4}}}`, true},
		{"block state", `{"description":"stairs","until":{"built":{"blocks":[{"pos":{"x":1,"y":2,"z":3},"block":"minecraft:oak_stairs[facing=north]"}]}}}`, true},
		{"zero count", `{"description":"x","until":{"items":{"item":"minecraft:oak_log","count":0}}}`, false},
		{"command injection", `{"description":"x","until":{"items":{"item":"dirt 0\nop clyde","count":1}}}`, false},
		{"fractional block pos", `{"description":"x","until":{"delivered":{"container":{"x":1.5,"y":2,"z":3},"item":"minecraft:dirt","count":1}}}`, false},
		{"missing until", `{"description":"x"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var g Goal
			err := json.Unmarshal([]byte(tt.json), &g)
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

// fakeConsole replays replies captured from Paper 26.1.2.
type fakeConsole map[string]string

func (f fakeConsole) Exec(cmd string) (string, error) {
	out, ok := f[cmd]
	if !ok {
		return "", errors.New("unexpected command " + cmd)
	}
	return out, nil
}

func TestRCONObserver(t *testing.T) {
	ctx := context.Background()
	chest := BlockPos{X: -307, Y: 79, Z: 151}
	obs := RCON{Conn: fakeConsole{
		"clear clyde minecraft:oak_log 0":                                  "Found 3 matching item(s) on player clyde",
		"clear clyde minecraft:dirt 0":                                     "No items were found on player clyde",
		"clear ghost minecraft:dirt 0":                                     "No player was found",
		"data get entity clyde Pos":                                        "clyde has the following entity data: [-309.5d, 79.0d, 151.5d]",
		"data get entity ghost Pos":                                        "No entity was found",
		"execute if block -307 79 151 minecraft:crafting_table":            "Test passed",
		"execute if block -307 79 151 minecraft:chest":                     "Test failed",
		"execute if block -307 999 151 minecraft:chest":                    "That position is out of this world!",
		"execute if items block -307 79 151 container.* minecraft:oak_log": "Test passed. Count: 67",
		"execute if items block -307 79 151 container.* minecraft:dirt":    "Test failed",
		"execute if items block -306 79 151 container.* minecraft:dirt":    "Source position -306, 79, 151 is not a container",
	}}

	for item, want := range map[ItemID]int{"minecraft:oak_log": 3, "minecraft:dirt": 0} {
		if n, err := obs.ItemCount(ctx, "clyde", item); err != nil || n != want {
			t.Errorf("ItemCount(%s) = %d, %v; want %d", item, n, err, want)
		}
	}
	if n, err := obs.ItemCount(ctx, "ghost", "minecraft:dirt"); err != nil || n != 0 {
		t.Errorf("ItemCount offline = %d, %v; want 0", n, err)
	}
	if p, ok, err := obs.Position(ctx, "clyde"); err != nil || !ok || p != (Vec3{X: -309.5, Y: 79, Z: 151.5}) {
		t.Errorf("Position = %v, %v, %v", p, ok, err)
	}
	if _, ok, err := obs.Position(ctx, "ghost"); err != nil || ok {
		t.Errorf("Position offline = %v, %v; want not ok", ok, err)
	}
	if ok, err := obs.BlockMatches(ctx, chest, "minecraft:crafting_table"); err != nil || !ok {
		t.Errorf("BlockMatches pass = %v, %v", ok, err)
	}
	if ok, err := obs.BlockMatches(ctx, chest, "minecraft:chest"); err != nil || ok {
		t.Errorf("BlockMatches fail = %v, %v", ok, err)
	}
	if _, err := obs.BlockMatches(ctx, BlockPos{X: -307, Y: 999, Z: 151}, "minecraft:chest"); err == nil {
		t.Error("BlockMatches out of world: want error")
	}
	if n, err := obs.ContainerCount(ctx, chest, "minecraft:oak_log"); err != nil || n != 67 {
		t.Errorf("ContainerCount = %d, %v; want 67", n, err)
	}
	if n, err := obs.ContainerCount(ctx, chest, "minecraft:dirt"); err != nil || n != 0 {
		t.Errorf("ContainerCount empty = %d, %v; want 0", n, err)
	}
	if _, err := obs.ContainerCount(ctx, BlockPos{X: -306, Y: 79, Z: 151}, "minecraft:dirt"); err == nil || !strings.Contains(err.Error(), "not a container") {
		t.Errorf("ContainerCount non-container: got %v", err)
	}
	if _, err := obs.ItemCount(ctx, "clyde @a", "minecraft:dirt"); err == nil {
		t.Error("bad player name: want error")
	}
	if _, err := obs.ItemCount(ctx, "clyde", "minecraft:dirt 0\nop clyde"); err == nil {
		t.Error("item with spaces: want error")
	}
}
