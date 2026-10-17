package gym_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/lox/clyde/harness/gym"
	"github.com/lox/clyde/harness/skills"
)

func TestFailedGatePreservesLiveSkill(t *testing.T) {
	candidates := skills.Store{Dir: t.TempDir()}
	dir := filepath.Join(candidates.Dir, "test-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"skill.md": "test", "index.js": "return true", "test.yaml": `name: test
template: mcbot
timeout: 1s
skill: {args: {}}
goal:
  description: arrive
  until:
    arrived: {at: {x: 1, y: 2, z: 3}, radius: 1}
`}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	k, err := candidates.LoadCandidate("test-skill")
	if err != nil {
		t.Fatal(err)
	}
	if err := candidates.RecordGate(k.Name, k.Digest, true, "prior pass"); err != nil {
		t.Fatal(err)
	}
	live := skills.Store{Dir: t.TempDir()}
	if err := live.Promote(candidates, k.Name); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "gym", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Offline lifecycle failure: no Java, server, or provider involved.
	if err := os.WriteFile(filepath.Join(root, "gym", "bin", "paper"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	runs := t.TempDir()
	if err := os.MkdirAll(filepath.Join(runs, "test"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runs, "test", "outcome.json"), []byte(`{"tokens":{"total":999},"skill_calls":{"stale":1}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := gym.Gate(context.Background(), candidates, k.Name, gym.Runner{Root: root, RunDir: runs, Log: io.Discard})
	if err != nil || res.Result != "error" {
		t.Fatalf("gate: %+v %v", res, err)
	}
	if res.Tokens != 0 || len(res.SkillCalls) != 0 {
		t.Fatalf("stale usage reused: %+v", res)
	}
	if _, err := candidates.Load(k.Name); err == nil {
		t.Fatal("failed gate admitted candidate")
	}
	if err := live.Promote(candidates, k.Name); err == nil {
		t.Fatal("failed gate allowed promotion")
	}
	if got, err := live.Load(k.Name); err != nil || got.Digest != k.Digest {
		t.Fatalf("live changed: %+v %v", got, err)
	}
}
