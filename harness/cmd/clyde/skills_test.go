package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lox/clyde/harness/skills"
)

func TestSkillVerify(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"skill.md", "index.js", "test.yaml"} {
		if err := os.WriteFile(filepath.Join(path, file), []byte("example"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	verify := func() error { return skillCmd(context.Background(), []string{"verify", "--dir", dir, "example"}) }
	if err := verify(); err == nil {
		t.Fatal("unadmitted skill accepted")
	}
	store := skills.Store{Dir: dir}
	k, err := store.LoadCandidate("example")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordGate(k.Name, k.Digest, true, "test fixture only"); err != nil {
		t.Fatal(err)
	}
	if err := verify(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "index.js"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verify(); err == nil {
		t.Fatal("stale admission accepted")
	}
}
