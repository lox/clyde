package planner_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lox/clyde/harness/planner"
	"github.com/lox/clyde/harness/skills"
)

func TestCandidatesAreSeparateImmutableAndUnadmitted(t *testing.T) {
	dir := t.TempDir()
	live, root := filepath.Join(dir, "live"), filepath.Join(dir, "candidates")
	stage, err := planner.NewCandidates(root, live)
	if err != nil {
		t.Fatal(err)
	}
	c := planner.Candidate{Name: "craft-probe", Description: "Craft a test item", Code: "return 'not evidence';", Test: `name: test
template: mcbot
timeout: 2s
setup: ["clear {player}"]
skill:
  args: {}
goal:
  description: Obtain a diamond
  until:
    items: {item: minecraft:diamond, count: 1}
`}
	first, err := stage.Save(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	c.Code = "throw new Error('revision');"
	second, err := stage.Save(context.Background(), c)
	if err != nil || second == first {
		t.Fatalf("revision path=%s error=%v", second, err)
	}
	store := skills.Store{Dir: filepath.Dir(first)}
	k, err := store.LoadCandidate(c.Name)
	if err != nil || k.Code != "return 'not evidence';" {
		t.Fatalf("original overwritten: %+v %v", k, err)
	}
	if _, err := store.Load(c.Name); err == nil {
		t.Fatal("play admitted an ungated candidate")
	}
	if err := (skills.Store{Dir: live}).Promote(store, c.Name); err == nil {
		t.Fatal("promoted ungated candidate")
	}
	if _, err := os.Stat(filepath.Join(live, c.Name)); !os.IsNotExist(err) {
		t.Fatal("failed promotion touched live installation")
	}
	if _, err := planner.NewCandidates(filepath.Join(live, "nested"), live); err == nil {
		t.Fatal("accepted candidate root inside live library")
	}
	if err := os.Symlink(live, filepath.Join(dir, "alias")); err != nil {
		t.Fatal(err)
	}
	if _, err := planner.NewCandidates(filepath.Join(dir, "alias"), live); err == nil {
		t.Fatal("accepted symlink alias for live library")
	}
	c.Name = "../escape"
	if _, err := stage.Save(context.Background(), c); err == nil {
		t.Fatal("candidate path escaped root")
	}
}
