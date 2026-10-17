// Package mcp exposes Clyde's body tools over the Model Context Protocol,
// so an Amp thread or any MCP client can drive the bot directly. The tools
// are the executor's, adapted; run_goal and stop are added here.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lox/clyde/harness/body"
	"github.com/lox/clyde/harness/executor"
	"github.com/lox/clyde/harness/goal"
)

// NewServer returns an MCP server with one MCP tool per executor tool.
func NewServer(version string, tools []executor.Tool) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "clyde", Version: version}, nil)
	for _, t := range tools {
		s.AddTool(&mcp.Tool{Name: t.Name, Description: t.Description, InputSchema: t.Schema}, handler(t))
	}
	return s
}

// handler reports tool failures as tool errors the client's model can read,
// and only a cancelled request as a protocol error.
func handler(t executor.Tool) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := t.Run(ctx, req.Params.Arguments)
		if err != nil {
			if executor.IsCancel(ctx, err) {
				return nil, err
			}
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "error: " + err.Error()}}}, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil
	}
}

// StopTool aborts the running action and halts the bot.
func StopTool(c *body.Client) executor.Tool {
	return executor.Tool{
		Name:        "stop",
		Description: "Abort Clyde's running action and stand still (POST /stop).",
		Schema:      json.RawMessage(`{"type":"object","properties":{}}`),
		Run: func(ctx context.Context, _ json.RawMessage) (string, error) {
			if err := c.Stop(ctx); err != nil {
				return "", err
			}
			return "stopped", nil
		},
	}
}

// RunGoalTool runs one goal through the executor and returns its outcome.
func RunGoalTool(run func(context.Context, goal.Goal) (executor.Outcome, error)) executor.Tool {
	return executor.Tool{
		Name: "run_goal",
		Description: "Hand Clyde a typed goal and let the executor model pursue it until its until predicate holds, " +
			"the judge interrupts, or the budget runs out. Returns the outcome with steps, tokens, and cost. " +
			`Example: {"goal": {"description": "Collect 4 oak logs.", "until": {"items": {"item": "minecraft:oak_log", "count": 4}}}}. ` +
			"Predicates (exactly one): items {item, count}; arrived {at: {x, y, z}, radius}; built {blocks: [{pos, block}]}; delivered {container, item, count}.",
		Schema: json.RawMessage(`{"type":"object","required":["goal"],"properties":{"goal":{"type":"object","description":"A goal per api/goal.schema.json: {description, until}."}}}`),
		Run: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct {
				Goal json.RawMessage `json:"goal"`
			}
			if err := json.Unmarshal(args, &a); err != nil || len(a.Goal) == 0 {
				return "", fmt.Errorf("goal is required")
			}
			var g goal.Goal
			if err := json.Unmarshal(a.Goal, &g); err != nil {
				return "", fmt.Errorf("invalid goal: %w", err)
			}
			if _, err := goal.Kind(g.Until); err != nil {
				return "", err
			}
			out, err := run(ctx, g)
			if err != nil {
				return "", err
			}
			b, err := json.MarshalIndent(out, "", "  ")
			return string(b), err
		},
	}
}
