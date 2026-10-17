package memory_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lox/clyde/harness/memory"
)

func TestPersistenceAndSemanticBoundary(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path, notes := filepath.Join(dir, "episodes.db"), filepath.Join(dir, "notes")
	s, err := memory.Open(path, notes)
	if err != nil {
		t.Fatal(err)
	}
	eps := []memory.Episode{{Kind: "goal_failed", Detail: "fell while gathering", Tokens: 17, CostUSD: 0.004}}
	if err := s.Commit(ctx, map[string]int{"cursor": 27}, eps, "event-27"); err != nil {
		t.Fatal(err)
	}
	p := memory.Place{Name: "player:Alex", Kind: "player", Dimension: "the_nether", X: 31, Y: 72, Z: -4, At: time.Now().UTC()}
	if err := s.Observe(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := s.Curate(ctx, "lesson", "Check the ledge before gathering.", []int64{eps[0].ID}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../escape", "/absolute", "UPPER"} {
		if err := s.Curate(ctx, name, "bad", []int64{eps[0].ID}); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	if err := s.Curate(ctx, "fabricated", "bad", []int64{9999}); err == nil {
		t.Fatal("accepted invented supporting episode")
	}
	if err := s.Curate(ctx, "oversized", strings.Repeat("x", memory.MaxNoteBytes), []int64{eps[0].ID}); err == nil {
		t.Fatal("accepted oversized note")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = memory.Open(path, notes)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var checkpoint map[string]int
	if err := s.Load(ctx, &checkpoint); err != nil || checkpoint["cursor"] != 27 {
		t.Fatalf("checkpoint=%v error=%v", checkpoint, err)
	}
	seen, err := s.Seen(ctx, "event-27")
	if err != nil || !seen {
		t.Fatalf("event not retained: %v", err)
	}
	got, err := s.Recent(ctx, 1)
	if err != nil || len(got) != 1 || got[0].Tokens != 17 || got[0].CostUSD != 0.004 {
		t.Fatalf("episodes=%+v error=%v", got, err)
	}
	places, err := s.Places(ctx)
	if err != nil || len(places) != 1 || places[0] != p {
		t.Fatalf("places=%+v error=%v", places, err)
	}
	text, err := s.Semantic()
	if err != nil || !strings.Contains(text, "Supporting episodes: 1") || !strings.Contains(text, "Check the ledge") {
		t.Fatalf("semantic=%q error=%v", text, err)
	}
	// A poisoned on-disk note must not read outside the configured directory.
	if err := os.WriteFile(filepath.Join(dir, "outside"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../outside", filepath.Join(notes, "escape.md")); err != nil {
		t.Fatal(err)
	}
	if text, err := s.Semantic(); err == nil || strings.Contains(text, "private") {
		t.Fatal("read escaped semantic directory")
	}
}

func TestAtomicCommitAndConcurrentReaders(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := memory.Open(filepath.Join(dir, "memory.db"), filepath.Join(dir, "notes"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	eps := []memory.Episode{{Kind: "start", Detail: "initial"}}
	if err := s.Commit(ctx, 1, eps, "existing"); err != nil {
		t.Fatal(err)
	}
	if err := s.Commit(ctx, 2, []memory.Episode{{Kind: "bad", Detail: "must roll back"}}, "existing"); err == nil {
		t.Fatal("duplicate event commit should fail")
	}
	var state int
	if err := s.Load(ctx, &state); err != nil || state != 1 {
		t.Fatalf("partial checkpoint commit: %d %v", state, err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 8 {
				if err := s.Curate(ctx, "lesson", "Bounded lesson.", []int64{1}); err != nil {
					t.Error(err)
				}
				if _, err := s.Semantic(); err != nil {
					t.Error(err)
				}
				if _, err := s.Recent(ctx, 8); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	got, err := s.Recent(ctx, 32)
	if err != nil || len(got) != 1 {
		t.Fatalf("partial episode commit: %+v %v", got, err)
	}
}
