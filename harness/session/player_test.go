package session_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agent "github.com/lox/agent-harness"
	"github.com/lox/clyde/harness/body"
	"github.com/lox/clyde/harness/session"
)

func TestRestartDoesNotReplayAmbiguousTools(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "player.db")
	j, err := session.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := j.Initialize(ctx, 4)
	if err != nil {
		t.Fatal(err)
	}
	assistant := agent.Message{Role: agent.RoleAssistant, ProviderData: json.RawMessage(`{"native":"unchanged"}`), ToolCalls: []agent.ToolCall{
		{ID: "chat", Name: "act", Arguments: json.RawMessage(`{"type":"chat","message":"working"}`)},
		{ID: "walk", Name: "act", Arguments: json.RawMessage(`{"type":"goto","x":17}`)},
	}}
	s.State.Messages = []agent.Message{assistant}
	s.State.Pending = []session.PendingTool{{Call: assistant.ToolCalls[0], Started: true}, {Call: assistant.ToolCalls[1]}}
	s.State.ActionToken = "stale-action"
	s.State.Wait = &session.Wait{Until: time.Now().UTC().Add(time.Minute), Reason: "waiting"}
	if _, err := j.Commit(ctx, s.Revision, session.Update{State: s.State, Entries: []session.Entry{messageEntry(t, assistant)}}); err != nil {
		t.Fatal(err)
	}
	if err := j.Control(ctx, true, 0); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = session.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	var mutations atomic.Int64
	var failStop atomic.Bool
	var failRead atomic.Bool
	failStop.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stop":
			if failStop.Load() {
				http.Error(w, "stop unavailable", 503)
				return
			}
			w.Write([]byte(`{}`))
		case "/state":
			if failRead.Load() {
				http.Error(w, "state unavailable", 503)
				return
			}
			w.Write([]byte("health 20; inventory: birch_log 3"))
		default:
			mutations.Add(1)
			http.Error(w, "no effects permitted", 500)
		}
	}))
	defer server.Close()
	p := session.Player{Journal: j, Body: &body.Client{URL: server.URL}}
	if _, err := p.Reconcile(ctx); err == nil {
		t.Fatal("startup accepted a failed stop")
	}
	failed, err := j.Load(ctx)
	if err != nil || !reflect.DeepEqual(failed.State, s.State) {
		t.Fatalf("failed stop changed durable state: %+v %v", failed, err)
	}
	failStop.Store(false)
	failRead.Store(true)
	if _, err := p.Reconcile(ctx); err == nil {
		t.Fatal("startup accepted a failed observation")
	}
	failed, err = j.Load(ctx)
	if err != nil || !reflect.DeepEqual(failed.State, s.State) {
		t.Fatalf("failed observation changed durable state: %+v %v", failed, err)
	}
	failRead.Store(false)
	recovered, err := p.Reconcile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !recovered.Stopped || len(recovered.State.Pending) != 0 || recovered.State.ActionToken != "" || recovered.State.Wait != nil {
		t.Fatalf("recovery state=%+v", recovered)
	}
	messages := recovered.State.Messages
	if len(messages) != 4 || !reflect.DeepEqual(messages[0], assistant) {
		t.Fatalf("native assistant history changed: %+v", messages)
	}
	for i, want := range []struct{ id, text string }{{"chat", "outcome unknown"}, {"walk", "withheld"}} {
		result := messages[i+1].ToolResult
		if result == nil || result.ToolCallID != want.id || !result.IsError || !strings.Contains(result.Content, want.text) {
			t.Fatalf("result=%+v want=%+v", result, want)
		}
	}
	again, err := p.Reconcile(ctx)
	if err != nil || !reflect.DeepEqual(again.State.Messages, messages) || mutations.Load() != 0 {
		t.Fatalf("repeated or replayed recovery: %+v mutations=%d %v", again, mutations.Load(), err)
	}
	// A quiet wait survives the same production reconciliation path.
	wait := &session.Wait{Until: time.Now().UTC().Add(2 * time.Minute), Reason: "waiting for a reply"}
	again.State.Wait = wait
	if _, err := j.Commit(ctx, again.Revision, session.Update{State: again.State}); err != nil {
		t.Fatal(err)
	}
	quiet, err := p.Reconcile(ctx)
	if err != nil || !reflect.DeepEqual(quiet.State.Wait, wait) || !reflect.DeepEqual(quiet.State.Messages, messages) {
		t.Fatalf("restart woke a quiet wait: %+v %v", quiet, err)
	}
}
