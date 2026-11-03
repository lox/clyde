package session_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agent "github.com/lox/agent-harness"
	"github.com/lox/clyde/harness/memory"
	"github.com/lox/clyde/harness/session"
)

func messageEntry(t *testing.T, m agent.Message) session.Entry {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return session.Entry{Kind: "message", Data: raw}
}

func TestJournalRestartCompactionAndAtomicInbox(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "episodes.db")
	// The new journal shares the database, but must not replace legacy memory.
	legacy, err := memory.Open(path, filepath.Join(dir, "semantic"))
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	if err := legacy.Commit(ctx, map[string]string{"project": "unverified cabin"}, []memory.Episode{{Kind: "old", Detail: "legacy episode"}}); err != nil {
		t.Fatal(err)
	}
	j, err := session.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := j.Initialize(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	input := session.Input{Key: "body-1:chat-7", Kind: "chat", Text: "Use birch, not oak", At: time.Now().UTC()}
	if err := j.Accept(ctx, []session.Input{input, input}); err != nil {
		t.Fatal(err)
	}
	pending, err := j.Pending(ctx, 0, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	assistant := agent.Message{Role: agent.RoleAssistant, ProviderData: json.RawMessage(`{"opaque":{"signature":"unchanged"}}`), ToolCalls: []agent.ToolCall{
		{ID: "one", Name: "act", Arguments: json.RawMessage(`{"type":"chat","message":"on it"}`)},
		{ID: "two", Name: "act", Arguments: json.RawMessage(`{"type":"goto","x":17}`)},
	}}
	wake := time.Now().UTC().Add(3 * time.Minute)
	next := s.State
	next.Messages = []agent.Message{{Role: agent.RoleUser, Content: input.Text}, assistant,
		{Role: agent.RoleTool, ToolResult: &agent.ToolResult{ToolCallID: "one", Content: "outcome unknown after restart; not replayed", IsError: true}},
		{Role: agent.RoleTool, ToolResult: &agent.ToolResult{ToolCallID: "two", Content: "withheld; not dispatched", IsError: true}},
	}
	next.Wait = &session.Wait{Until: wake, Reason: "wait for the player"}
	entries := []session.Entry{}
	for _, m := range next.Messages {
		entries = append(entries, messageEntry(t, m))
	}
	s, err = j.Commit(ctx, s.Revision, session.Update{State: next, Entries: entries, Applied: []int64{pending[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	// A failed atomic publication must not consume events or add history.
	bad := next
	bad.Messages = []agent.Message{{Role: agent.RoleUser, Content: "must not publish"}}
	if _, err := j.Commit(ctx, s.Revision, session.Update{State: bad, Entries: []session.Entry{messageEntry(t, bad.Messages[0])}, Applied: []int64{999}}); err == nil {
		t.Fatal("accepted a missing inbox event")
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = session.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	restored, err := j.Initialize(ctx, 900)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.State, next) || restored.Limit != 3 {
		t.Fatalf("restart changed state/allowance: %+v", restored)
	}
	pending, err = j.Pending(ctx, 0, 10)
	if err != nil || len(pending) != 0 {
		t.Fatalf("reapplied events: %+v %v", pending, err)
	}
	if err := j.Accept(ctx, []session.Input{input}); err != nil {
		t.Fatal(err)
	}
	pending, err = j.Pending(ctx, 0, 10)
	if err != nil || len(pending) != 0 {
		t.Fatalf("duplicate after restart became pending: %+v %v", pending, err)
	}
	// A compacted active context never deletes or rewrites original records.
	compacted := next
	compacted.Messages = []agent.Message{{Role: agent.RoleUser, Content: "Working notes: birch cabin; history #1-4"}}
	if _, err := j.Commit(ctx, restored.Revision, session.Update{State: compacted}); err != nil {
		t.Fatal(err)
	}
	history, err := j.History(ctx, "", 0, 2)
	if err != nil || len(history) != 2 {
		t.Fatalf("history page=%+v %v", history, err)
	}
	var saved agent.Message
	if err := json.Unmarshal(history[1].Data, &saved); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved, assistant) {
		t.Fatalf("opaque message changed: %+v", saved)
	}
	last, err := j.History(ctx, "", history[1].ID, 10)
	if err != nil || len(last) != 2 {
		t.Fatalf("history suffix=%+v %v", last, err)
	}
	found, err := j.History(ctx, "birch", 0, 10)
	if err != nil || len(found) != 1 {
		t.Fatalf("search=%+v %v", found, err)
	}
	var old map[string]string
	if err := legacy.Load(ctx, &old); err != nil || old["project"] != "unverified cabin" {
		t.Fatalf("legacy checkpoint changed: %v %v", old, err)
	}
	eps, err := legacy.Recent(ctx, 10)
	if err != nil || len(eps) != 1 || eps[0].Detail != "legacy episode" {
		t.Fatalf("legacy episodes changed: %+v %v", eps, err)
	}
}

func TestDurableReservationsAndOperatorControls(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "player.db")
	j, err := session.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := j.Initialize(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	first, err := j.Reserve(ctx, "inference", "observed before dispatch")
	if err != nil {
		t.Fatal(err)
	}
	usage := agent.Usage{InputTokens: 31, OutputTokens: 7}
	s, err = j.Commit(ctx, s.Revision, session.Update{State: s.State, Receipt: &session.Receipt{ID: first, Usage: &usage}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Reserve(ctx, "compaction", "fresh observation"); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Reserve(ctx, "inference", ""); !errors.Is(err, session.ErrAllowance) {
		t.Fatalf("exhaustion=%v", err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = session.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	s, err = j.Initialize(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if s.Used != 2 || s.Limit != 2 {
		t.Fatalf("restart refilled allowance: %+v", s)
	}
	account, err := j.Accounting(ctx)
	if err != nil || account.Unknown != 1 || account.Usage != usage {
		t.Fatalf("accounting=%+v %v", account, err)
	}
	// A second connection models a local CLI, not a model-callable tool.
	operator, err := session.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer operator.Close()
	if err := operator.Control(ctx, true, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Reserve(ctx, "inference", ""); !errors.Is(err, session.ErrStopped) {
		t.Fatalf("stopped reserve=%v", err)
	}
	if _, err := j.Commit(ctx, s.Revision, session.Update{State: s.State, Dispatch: true}); !errors.Is(err, session.ErrStopped) {
		t.Fatalf("stale checkpoint dispatched after operator stop: %v", err)
	}
	// A stale runtime checkpoint cannot overwrite an operator stop or extension.
	s, err = j.Commit(ctx, s.Revision, session.Update{State: s.State})
	if err != nil || !s.Stopped || s.Limit != 5 || s.Used != 2 {
		t.Fatalf("lost operator control: %+v %v", s, err)
	}
	if err := operator.Control(ctx, false, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Reserve(ctx, "inference", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Commit(ctx, s.Revision-1, session.Update{State: s.State}); !errors.Is(err, session.ErrConflict) {
		t.Fatalf("stale checkpoint=%v", err)
	}
	if err := operator.Control(ctx, true, 0); err != nil {
		t.Fatal(err)
	}
	if err := operator.Close(); err != nil {
		t.Fatal(err)
	}
	operator, err = session.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer operator.Close()
	if _, err := operator.Reserve(ctx, "inference", ""); !errors.Is(err, session.ErrStopped) {
		t.Fatalf("restart cleared operator stop: %v", err)
	}
}

func TestConcurrentReservationsCannotOverdraw(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "player.db")
	var journals []*session.Journal
	for range 2 {
		j, err := session.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer j.Close()
		journals = append(journals, j)
	}
	if _, err := journals[0].Initialize(ctx, 7); err != nil {
		t.Fatal(err)
	}
	var granted atomic.Int64
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			_, err := journals[i%len(journals)].Reserve(ctx, "inference", "same allowance")
			if err == nil {
				granted.Add(1)
			} else if !errors.Is(err, session.ErrAllowance) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	s, err := journals[0].Load(ctx)
	if err != nil || granted.Load() != 7 || s.Used != 7 {
		t.Fatalf("granted=%d state=%+v err=%v", granted.Load(), s, err)
	}
	a, err := journals[0].Accounting(ctx)
	if err != nil || a.Unknown != 7 {
		t.Fatalf("lost unreceipted reservations: %+v %v", a, err)
	}
}

func TestIncompatibleJournalIsRefused(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "player.db")
	j, err := session.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Initialize(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	// A fixture written by a future binary must not be interpreted as our state.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `UPDATE player_state SET version=999`); err != nil {
		t.Fatal(err)
	}
	if other, err := session.Open(path); err == nil {
		other.Close()
		t.Fatal("accepted incompatible version")
	} else if !strings.Contains(err.Error(), "unsupported journal version 999") {
		t.Fatal(err)
	}
}
