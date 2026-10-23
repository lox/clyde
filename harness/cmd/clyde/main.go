// Command clyde drives Clyde's body with the executor.
//
//	clyde run --goal goal.json   run one goal; prints the outcome as JSON
//	clyde mcp                    serve the body tools over MCP (stdio, or --http)
//	clyde serve                  run the durable planner and player-first scheduler
//
// Both talk to a running body (--body, default $CLYDE_BODY_URL or
// http://127.0.0.1:7777). The model comes from CLYDE_EXECUTOR_PROVIDER,
// CLYDE_EXECUTOR_MODEL, and CLYDE_EXECUTOR_EFFORT (see harness/llm).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lox/clyde/harness/body"
	"github.com/lox/clyde/harness/executor"
	"github.com/lox/clyde/harness/goal"
	"github.com/lox/clyde/harness/gym"
	"github.com/lox/clyde/harness/jev"
	"github.com/lox/clyde/harness/judge"
	"github.com/lox/clyde/harness/llm"
	"github.com/lox/clyde/harness/mcp"
	"github.com/lox/clyde/harness/rcon"
)

const version = "0.4.0"

// exitNotSucceeded is the exit status of `clyde run` when the goal was not
// reached; 1 means clyde itself failed.
const exitNotSucceeded = 2

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if len(os.Args) < 2 || (os.Args[1] != "run" && os.Args[1] != "mcp" && os.Args[1] != "skill" && os.Args[1] != "serve") {
		fmt.Fprintln(os.Stderr, "usage: clyde run --goal goal.json [flags] | clyde mcp [flags] | clyde serve [flags] | clyde skill gate|promote|verify [flags] name...")
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	if cmd == "run" {
		err = runCmd(ctx, args)
	} else if cmd == "skill" {
		err = skillCmd(ctx, args)
	} else if cmd == "serve" {
		err = serveCmd(ctx, args)
	} else {
		err = mcpCmd(ctx, args)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "clyde:", err)
		os.Exit(1)
	}
}

// common holds what both subcommands need to run goals.
type common struct {
	root      string
	bodyURL   string
	rconAddr  string
	judge     string
	judgeLog  string
	maxSteps  int
	maxTime   time.Duration
	skillsDir string
}

func (c *common) flags(fs *flag.FlagSet) {
	c.root = repoRoot()
	bodyURL := os.Getenv("CLYDE_BODY_URL")
	if bodyURL == "" {
		bodyURL = "http://127.0.0.1:7777"
	}
	fs.StringVar(&c.bodyURL, "body", bodyURL, "body base URL")
	fs.StringVar(&c.rconAddr, "rcon", "", "RCON address for checking until predicates (password from GYM_RCON_PASSWORD); without it the bot's own card answers items and arrived only")
	fs.StringVar(&c.judge, "judge", "auto", "judge backend: jev, off, or auto (jev when TYPESAFE_API_KEY is set)")
	fs.StringVar(&c.judgeLog, "judge-log", filepath.Join(c.root, "memory", "judge.jsonl"), "JSONL judge tick log to append to")
	fs.IntVar(&c.maxSteps, "max-steps", 40, "model turns per goal")
	fs.DurationVar(&c.maxTime, "max-time", 5*time.Minute, "wall time per goal")
	fs.StringVar(&c.skillsDir, "skills-dir", filepath.Join(c.root, "skills"), "admitted library root; empty disables skill tools")
}

// runGoal builds a fresh provider, observer, and judge, and runs g once.
func (c *common) runGoal(ctx context.Context, g goal.Goal, intent executor.ExecutionContext, log io.Writer) (executor.Outcome, error) {
	cfg := llm.ExecutorConfig()
	provider, err := llm.New(cfg)
	if err != nil {
		return executor.Outcome{}, err
	}
	system, err := os.ReadFile(filepath.Join(c.root, "prompts", "executor.md"))
	if err != nil {
		return executor.Outcome{}, err
	}
	client := &body.Client{URL: c.bodyURL}
	card, err := client.State(ctx)
	if err != nil {
		return executor.Outcome{}, fmt.Errorf("reading the body's state: %w", err)
	}
	o := executor.Options{
		Provider: provider, Model: cfg.Model, Effort: cfg.Effort, System: string(system),
		Context: intent,
		Body:    client, SkillsDir: c.skillsDir, Player: card.Bot.Name,
		Observer: body.Observer{Client: client}, MaxSteps: c.maxSteps, MaxDuration: c.maxTime, Log: log,
	}
	if c.rconAddr != "" {
		conn, err := rcon.Dial(ctx, c.rconAddr, gym.RCONPassword())
		if err != nil {
			return executor.Outcome{}, err
		}
		defer conn.Close()
		o.Observer = goal.RCON{Conn: conn}
	}
	if j, err := c.newJudge(log); err != nil {
		return executor.Outcome{}, err
	} else if j != nil {
		f, err := openLog(c.judgeLog)
		if err != nil {
			return executor.Outcome{}, err
		}
		defer f.Close()
		o.Judge, o.JudgeLog = j, f
	}
	fmt.Fprintf(log, "goal: %s (model %s, effort %s, judge %v)\n", g.Description, cfg.Model, cfg.Effort, o.Judge != nil)
	return executor.Run(ctx, g, o)
}

func (c *common) newJudge(log io.Writer) (*judge.Judge, error) {
	switch c.judge {
	case "off":
		return nil, nil
	case "auto":
		if os.Getenv("TYPESAFE_API_KEY") == "" {
			fmt.Fprintln(log, "judge: off (TYPESAFE_API_KEY is not set)")
			return nil, nil
		}
	case "jev":
	default:
		return nil, fmt.Errorf("unknown judge %q (want jev, off, or auto)", c.judge)
	}
	jc, err := jev.FromEnv()
	if err != nil {
		return nil, err
	}
	qs, err := judge.LoadQuestions(filepath.Join(c.root, "prompts", "judge.yaml"))
	if err != nil {
		return nil, err
	}
	return judge.New(judge.BodySource{URL: c.bodyURL}, judge.JevBackend{Client: jc}, qs, judge.DefaultConfig()), nil
}

func runCmd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("clyde run", flag.ExitOnError)
	var c common
	c.flags(fs)
	goalPath := fs.String("goal", "", "goal JSON file (api/goal.schema.json)")
	fs.Parse(args)
	if *goalPath == "" {
		return errors.New("--goal is required")
	}
	b, err := os.ReadFile(*goalPath)
	if err != nil {
		return err
	}
	var g goal.Goal
	if err := json.Unmarshal(b, &g); err != nil {
		return fmt.Errorf("%s: %w", *goalPath, err)
	}
	if _, err := goal.Kind(g.Until); err != nil {
		return fmt.Errorf("%s: %w", *goalPath, err)
	}
	lease, err := bodyLease(c.bodyURL)
	if err != nil {
		return err
	}
	defer lease.Close()
	out, err := c.runGoal(ctx, g, executor.ExecutionContext{}, os.Stderr)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return err
	}
	if out.Outcome != executor.Succeeded {
		os.Exit(exitNotSucceeded)
	}
	return nil
}

func mcpCmd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("clyde mcp", flag.ExitOnError)
	var c common
	c.flags(fs)
	httpAddr := fs.String("http", "", "serve streamable HTTP on this address instead of stdio, e.g. 127.0.0.1:7778")
	fs.Parse(args)
	lease, err := bodyLease(c.bodyURL)
	if err != nil {
		return err
	}
	defer lease.Close()

	client := &body.Client{URL: c.bodyURL}
	tb := &executor.Toolbox{Body: client, SkillsDir: c.skillsDir}
	tools := append(tb.Tools(),
		mcp.StopTool(client),
		mcp.RunGoalTool(func(ctx context.Context, g goal.Goal) (executor.Outcome, error) {
			return c.runGoal(ctx, g, executor.ExecutionContext{}, os.Stderr)
		}),
	)
	server := mcp.NewServer(version, tools)
	if *httpAddr == "" {
		return server.Run(ctx, &sdk.StdioTransport{})
	}
	handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil)
	srv := &http.Server{Addr: *httpAddr, Handler: handler}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	fmt.Fprintf(os.Stderr, "clyde mcp: serving streamable HTTP on http://%s\n", *httpAddr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func openLog(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

// repoRoot walks up from the working directory to the checkout containing
// prompts/executor.md, so defaults work from the repo root or harness/. It
// falls back to ".".
func repoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "prompts", "executor.md")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}
