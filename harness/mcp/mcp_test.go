package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lox/clyde/harness/executor"
)

// Tool failures reach the client as isError results its model can read,
// not as protocol errors.
func TestToolErrorsAreResults(t *testing.T) {
	echo := executor.Tool{
		Name: "echo", Description: "echo", Schema: json.RawMessage(`{"type":"object","properties":{"say":{"type":"string"}}}`),
		Run: func(_ context.Context, args json.RawMessage) (string, error) {
			var a struct{ Say string }
			json.Unmarshal(args, &a)
			if a.Say == "" {
				return "", errors.New("say is required")
			}
			return a.Say, nil
		},
	}
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go NewServer("test", []executor.Tool{echo}).Run(ctx, st)
	sess, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	text := func(res *mcp.CallToolResult) string { return res.Content[0].(*mcp.TextContent).Text }
	ok, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"say": "hi"}})
	if err != nil || ok.IsError || text(ok) != "hi" {
		t.Fatalf("ok call: %+v %v", ok, err)
	}
	bad, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{}})
	if err != nil || !bad.IsError || text(bad) != "error: say is required" {
		t.Fatalf("bad call: %+v %v", bad, err)
	}
}
