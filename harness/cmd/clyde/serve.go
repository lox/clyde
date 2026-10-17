package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/lox/clyde/harness/body"
	"github.com/lox/clyde/harness/executor"
	"github.com/lox/clyde/harness/goal"
	"github.com/lox/clyde/harness/gym"
	"github.com/lox/clyde/harness/llm"
	"github.com/lox/clyde/harness/memory"
	"github.com/lox/clyde/harness/planner"
	"github.com/lox/clyde/harness/rcon"
)

func serveCmd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("clyde serve", flag.ContinueOnError)
	var c common
	c.flags(fs)
	dbPath := fs.String("memory", filepath.Join(c.root, "memory", "episodes.db"), "durable SQLite checkpoint/episode path")
	semanticDir := fs.String("semantic-dir", filepath.Join(c.root, "memory", "semantic"), "bounded Markdown memory directory")
	candidateDir := fs.String("candidates-dir", filepath.Join(c.root, "memory", "skill-candidates"), "staged proposals only; never the admitted library root")
	promptPath := fs.String("planner-prompt", filepath.Join(c.root, "prompts", "planner.md"), "planner system prompt")
	planningTime := fs.Duration("planner-time", 30*time.Second, "deadline for one planning event, including at most two attempts")
	maxAuto := fs.Int("max-autonomous", 10, "durable autonomous session goal budget; raise explicitly to continue after exhaustion")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 || *maxAuto < 1 || *planningTime <= 0 || c.maxSteps < 1 || c.maxTime <= 0 {
		return errors.New("serve: positive budgets and no positional arguments required")
	}
	lease, err := bodyLease(c.bodyURL)
	if err != nil {
		return err
	}
	defer lease.Close()
	absDB, err := filepath.Abs(*dbPath)
	if err != nil {
		return err
	}
	dbLease, err := localLease("memory:" + absDB)
	if err != nil {
		return err
	}
	defer dbLease.Close()
	store, err := memory.Open(*dbPath, *semanticDir)
	if err != nil {
		return err
	}
	defer store.Close()
	system, err := os.ReadFile(*promptPath)
	if err != nil {
		return err
	}
	format, err := os.ReadFile(filepath.Join(c.root, "skills", "README.md"))
	if err != nil {
		return err
	}
	schema, err := os.ReadFile(filepath.Join(c.root, "api", "goal.schema.json"))
	if err != nil {
		return err
	}
	cfg := planner.Config()
	provider, err := llm.New(cfg)
	if err != nil {
		return err
	}
	p, err := planner.New(planner.Options{Provider: provider, Config: cfg, System: string(system) + "\nTrusted skill format reference (commands are for humans only):\n" + string(format), GoalSchema: schema, Timeout: *planningTime})
	if err != nil {
		return err
	}
	liveDir := c.skillsDir
	if liveDir == "" {
		liveDir = filepath.Join(c.root, "skills")
	}
	stage, err := planner.NewCandidates(*candidateDir, liveDir)
	if err != nil {
		return err
	}
	client := &body.Client{URL: c.bodyURL}
	check := func(ctx context.Context, g goal.Goal) (bool, error) {
		card, err := client.State(ctx)
		if err != nil {
			return false, err
		}
		var obs goal.Observer = body.Observer{Client: client}
		if c.rconAddr != "" {
			conn, err := rcon.Dial(ctx, c.rconAddr, gym.RCONPassword())
			if err != nil {
				return false, err
			}
			defer conn.Close()
			obs = goal.RCON{Conn: conn}
		}
		result, err := goal.Evaluate(ctx, obs, card.Bot.Name, g.Until)
		return result.Satisfied, err
	}
	s := planner.Scheduler{Memory: store, World: client, Plan: p.Plan, Check: check, MaxAutonomous: *maxAuto, Log: os.Stderr,
		Execute: func(ctx context.Context, g goal.Goal) (executor.Outcome, error) { return c.runGoal(ctx, g, os.Stderr) },
		SaveCandidate: func(ctx context.Context, k planner.Candidate) error {
			path, err := stage.Save(ctx, k)
			if err == nil {
				fmt.Fprintf(os.Stderr, "candidate staged (not admitted): %s\n", path)
			}
			return err
		},
	}
	fmt.Fprintf(os.Stderr, "clyde serve: planner=%s effort=%s executor=%s memory=%s\n", cfg.Model, cfg.Effort, llm.ExecutorConfig().Model, absDB)
	return s.Run(ctx)
}

// Local advisory leases prevent accidentally running serve beside another local
// clyde run/mcp/serve owner. Remote controllers and aliases for a remote host
// remain an operator responsibility; this is not a distributed body lease.
func bodyLease(raw string) (*os.File, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, errors.New("invalid body URL")
	}
	if u.Hostname() == "localhost" {
		u.Host = strings.Replace(u.Host, "localhost", "127.0.0.1", 1)
	}
	return localLease("body:" + strings.TrimRight(u.String(), "/"))
}

func localLease(key string) (*os.File, error) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("clyde-%x.lock", sha256.Sum256([]byte(key))))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another local Clyde owns %s", key)
	}
	return f, nil // close releases the lock, including on process death; do not unlink
}
