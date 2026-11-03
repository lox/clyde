package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lox/clyde/harness/memory"
	"github.com/lox/clyde/harness/session"
)

func TestBodyOwnershipAndServeBudgetValidation(t *testing.T) {
	u := fmt.Sprintf("http://localhost:7777/%s", t.Name())
	f, err := bodyLease(u)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := bodyLease(strings.Replace(u, "localhost", "127.0.0.1", 1) + "/"); err == nil {
		other.Close()
		t.Fatal("second controller acquired body")
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	f, err = bodyLease(u)
	if err != nil {
		t.Fatal("ownership not released:", err)
	}
	f.Close()
	for _, args := range [][]string{{"--max-autonomous", "0"}, {"--planner-time", "0"}, {"--max-steps", "40"}, {"--max-time", "5m"}, {"--request-limit", "0"}, {"--call-timeout", "0"}, {"--max-output-tokens", "0"}, {"--context-bytes", "100"}, {"unexpected"}} {
		if err := serveCmd(context.Background(), args); err == nil {
			t.Fatalf("accepted invalid serve arguments %v", args)
		}
	}
}

func TestServeImportsCopiedLegacyMemoryWithoutReplaying(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	legacy, err := memory.Open(filepath.Join(dir, "old.db"), filepath.Join(dir, "semantic"))
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := map[string]any{"project": map[string]string{"intent": "birch workshop"}, "jobs": []map[string]string{{"description": "pending delivery"}}, "recovery": map[string]any{"closed": true, "used": 2}}
	if err := legacy.Commit(ctx, checkpoint, []memory.Episode{{Kind: "old", Detail: "retained episode"}}); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "copy.db")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	j, err := session.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if _, err := j.Initialize(ctx, 3); err != nil {
		t.Fatal(err)
	}
	if err := j.Control(ctx, true, 0); err != nil {
		t.Fatal(err)
	}
	var effects atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stop":
			fmt.Fprint(w, `{}`)
		case "/events":
			fmt.Fprint(w, `{"events":[],"latestSeq":0}`)
		case "/state":
			if r.URL.Query().Get("format") == "text" {
				fmt.Fprint(w, "health20; no running action")
				return
			}
			fmt.Fprint(w, `{"ts":1,"bot":{"name":"Clyde","dimension":"overworld","position":{"x":0,"y":80,"z":0},"yaw":0,"pitch":0,"health":20,"food":20,"oxygen":20,"xpLevel":0,"gameMode":"survival","onGround":true,"inWater":false,"inLava":false},"world":{"timeOfDay":6000,"phase":"day","raining":false,"thundering":false},"action":null,"lastAction":null,"inventory":{"used":0,"total":36,"held":null,"items":[]},"entities":{"players":[],"hostiles":[],"passives":[],"items":[]},"blocks":[],"events":[]}`)
		default:
			effects.Add(1)
			http.Error(w, "no replay", 500)
		}
	}))
	defer srv.Close()
	t.Setenv("OPENAI_API_KEY", "fixture-not-used")
	t.Setenv("CLYDE_EXECUTOR_PROVIDER", "openai")
	for _, name := range []string{"CLYDE_PLANNER_PROVIDER", "CLYDE_PLANNER_MODEL", "CLYDE_PLANNER_EFFORT"} {
		t.Setenv(name, "")
	}
	for range 2 {
		call, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		err := serveCmd(call, []string{"--body", srv.URL, "--memory", path, "--semantic-dir", filepath.Join(dir, "semantic"), "--candidates-dir", filepath.Join(dir, "candidates"), "--judge", "off"})
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	}
	s, err := j.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Stopped || s.Used != 0 || effects.Load() != 0 || len(s.State.Messages) != 1 {
		t.Fatalf("migration replayed/refilled/duplicated: %+v effects=%d", s, effects.Load())
	}
	text := s.State.Messages[0].Content
	if !strings.Contains(text, "unverified") || !strings.Contains(text, "birch workshop") || !strings.Contains(text, "pending delivery") {
		t.Fatalf("lost legacy commitments: %s", text)
	}
	copy, err := memory.Open(path, filepath.Join(dir, "semantic"))
	if err != nil {
		t.Fatal(err)
	}
	defer copy.Close()
	var retained map[string]any
	if err := copy.Load(ctx, &retained); err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(checkpoint)
	got, _ := json.Marshal(retained)
	if string(got) != string(want) {
		t.Fatalf("legacy state changed: %s", got)
	}
	episodes, err := copy.Recent(ctx, 10)
	if err != nil || len(episodes) != 1 || episodes[0].Detail != "retained episode" {
		t.Fatalf("legacy episodes lost: %+v %v", episodes, err)
	}
}
