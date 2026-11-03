package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	agent "github.com/lox/agent-harness"
	"github.com/lox/agent-harness/provider/openai"
)

// This is Clyde's durable compaction contract, exercised through the pinned
// provider's public API. A text-only or response-ID-only adapter fails it.
func TestNativeHistorySurvivesCompactionReset(t *testing.T) {
	requests := make(chan string, 3)
	n := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		requests <- string(raw)
		n++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"resp_%d","object":"response","status":"completed","output":[{"type":"reasoning","id":"rs_original","summary":[],"encrypted_content":"opaque_fixture"},{"type":"function_call","id":"fc_original","call_id":"call_original","name":"observe","arguments":"{}","status":"completed"}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}`, n)
	}))
	defer server.Close()
	p := openai.New(openai.WithAPIKey("fixture-only"), openai.WithBaseURL(server.URL), openai.WithDefaultModel(DefaultModel))
	first, err := p.Chat(context.Background(), agent.ChatParams{Messages: []agent.Message{{Role: agent.RoleUser, Content: "project"}}})
	if err != nil {
		t.Fatal(err)
	}
	if request := <-requests; !strings.Contains(request, "reasoning.encrypted_content") {
		t.Fatalf("first response is not replayable: %s", request)
	}
	raw, err := json.Marshal(first.Message)
	if err != nil {
		t.Fatal(err)
	}
	var restored agent.Message
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(restored.ProviderData), "opaque_fixture") {
		t.Fatalf("lost native history: %s", raw)
	}
	messages := []agent.Message{
		{Role: agent.RoleUser, Content: "COMPACTED_NOTES"}, restored,
		{Role: agent.RoleTool, ToolResult: &agent.ToolResult{ToolCallID: "call_original", Content: "observed"}},
	}
	second, err := p.Chat(context.Background(), agent.ChatParams{Messages: messages, Options: map[string]any{"previous_response_id": ""}})
	if err != nil {
		t.Fatal(err)
	}
	request := <-requests
	if strings.Contains(request, "previous_response_id") || !strings.Contains(request, "COMPACTED_NOTES") || !strings.Contains(request, "opaque_fixture") || !strings.Contains(request, "function_call_output") {
		t.Fatalf("invalid reset: %s", request)
	}
	messages = append(messages, second.Message, agent.Message{Role: agent.RoleTool, ToolResult: &agent.ToolResult{ToolCallID: "call_original", Content: "latest observation"}})
	if _, err := p.Chat(context.Background(), agent.ChatParams{Messages: messages}); err != nil {
		t.Fatal(err)
	}
	request = <-requests
	if !strings.Contains(request, `"previous_response_id":"resp_2"`) || strings.Contains(request, "COMPACTED_NOTES") {
		t.Fatalf("did not resume new anchor: %s", request)
	}
}

func TestPersistentProviderDoesNotHideRetries(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"message":"rate limited","type":"rate_limit_error"}}`)
	}))
	defer server.Close()
	t.Setenv("OPENAI_API_KEY", "fixture-only")
	t.Setenv("OPENAI_BASE_URL", server.URL)
	p, err := New(Config{Provider: DefaultProvider, Model: DefaultModel, NoRetries: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Chat(context.Background(), agent.ChatParams{Messages: []agent.Message{{Role: agent.RoleUser, Content: "bounded request"}}}); err == nil {
		t.Fatal("rate limit should be returned to the session")
	}
	if n := requests.Load(); n != 1 {
		t.Fatalf("one reservation dispatched %d HTTP requests", n)
	}
}
