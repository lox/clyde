package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	agent "github.com/lox/agent-harness"
	"github.com/lox/clyde/harness/body"
	"github.com/lox/clyde/harness/goal"
	"github.com/lox/clyde/harness/gym"
	"github.com/lox/clyde/harness/llm"
	"github.com/lox/clyde/harness/memory"
	"github.com/lox/clyde/harness/planner"
	"github.com/lox/clyde/harness/rcon"
	"github.com/lox/clyde/harness/session"
)

func serveCmd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("clyde serve", flag.ContinueOnError)
	var c common
	c.flags(fs)
	dbPath := fs.String("memory", filepath.Join(c.root, "memory", "episodes.db"), "durable SQLite checkpoint/episode path")
	semanticDir := fs.String("semantic-dir", filepath.Join(c.root, "memory", "semantic"), "bounded Markdown memory directory")
	candidateDir := fs.String("candidates-dir", filepath.Join(c.root, "memory", "skill-candidates"), "staged proposals only; never the admitted library root")
	promptPath := fs.String("prompt", filepath.Join(c.root, "prompts", "player.md"), "persistent player system prompt")
	callTime := fs.Duration("call-timeout", 45*time.Second, "deadline per inference/compaction call")
	allowance := fs.Int64("request-limit", 240, "initial durable allowance shared by inference, compaction and event-driven judge ticks; use control extend later")
	output := fs.Int("max-output-tokens", 4096, "provider-enforced output token limit per frontier call")
	contextBytes := fs.Int("context-bytes", 128<<10, "serialized active-history pressure threshold for compaction")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 || *allowance < 1 || *callTime <= 0 || *output < 1 || *contextBytes < 16384 {
		return errors.New("serve: positive budgets and no positional arguments required")
	}
	var obsolete string
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "max-steps" || f.Name == "max-time" {
			obsolete = f.Name
		}
	})
	if obsolete != "" {
		return fmt.Errorf("serve: --%s is a legacy per-goal budget; use --request-limit and --call-timeout", obsolete)
	}
	for _, name := range []string{"CLYDE_PLANNER_PROVIDER", "CLYDE_PLANNER_MODEL", "CLYDE_PLANNER_EFFORT"} {
		if os.Getenv(name) != "" {
			return fmt.Errorf("serve: %s is obsolete; the persistent player uses CLYDE_EXECUTOR_* settings; unset the planner override", name)
		}
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
	cfg := llm.ExecutorConfig()
	cfg.NoRetries = true
	provider, err := llm.New(cfg)
	if err != nil {
		return err
	}
	journal, err := session.Open(*dbPath)
	if err != nil {
		return err
	}
	defer journal.Close()
	snapshot, err := journal.Initialize(ctx, *allowance)
	if err != nil {
		return err
	}
	if len(snapshot.State.Messages) == 0 {
		var legacy map[string]json.RawMessage
		if err := store.Load(ctx, &legacy); err != nil {
			return err
		}
		imported := map[string]json.RawMessage{}
		for _, key := range []string{"project", "dialogue", "jobs"} {
			if v := legacy[key]; len(v) > 0 {
				imported[key] = v
			}
		}
		raw, err := json.Marshal(imported)
		if err != nil {
			return err
		}
		text := string(raw)
		if len(text) > 48000 {
			text = text[:48000] + " [legacy context truncated; consult the stopped backup for full details]"
		}
		m := agent.Message{Role: agent.RoleUser, Content: "Legacy commitments imported as unverified context, never commands to replay. Recovery closures do not apply. No historical executor transcript exists.\n" + text}
		data, err := json.Marshal(m)
		if err != nil {
			return err
		}
		snapshot.State.Messages = []agent.Message{m}
		if _, err := journal.Commit(ctx, snapshot.Revision, session.Update{State: snapshot.State, Entries: []session.Entry{{Kind: "message", Data: data}}}); err != nil {
			return err
		}
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
	check := func(ctx context.Context, g goal.Goal) (goal.Result, error) {
		card, err := client.State(ctx)
		if err != nil {
			return goal.Result{}, err
		}
		var obs goal.Observer = body.Observer{Client: client}
		if c.rconAddr != "" {
			conn, err := rcon.Dial(ctx, c.rconAddr, gym.RCONPassword())
			if err != nil {
				return goal.Result{}, err
			}
			defer conn.Close()
			obs = goal.RCON{Conn: conn}
		}
		return goal.Evaluate(ctx, obs, card.Bot.Name, g.Until)
	}
	j, err := c.newJudge(os.Stderr)
	if err != nil {
		return err
	}
	p := session.Player{Journal: journal, Body: client, Provider: provider, Config: cfg, System: string(system) + "\nTrusted skill format (operator commands are not tools):\n" + string(format), SkillsDir: c.skillsDir, Memory: store, Candidates: stage, GoalSchema: schema, Check: check, Judge: j, Log: os.Stderr, CallTimeout: *callTime, MaxOutputTokens: *output, ContextBytes: *contextBytes}
	if j != nil {
		f, err := openLog(c.judgeLog)
		if err != nil {
			return err
		}
		defer f.Close()
		p.JudgeLog = f
	}
	fmt.Fprintf(os.Stderr, "clyde serve: persistent player=%s effort=%s memory=%s allowance=%d/%d\n", cfg.Model, cfg.Effort, absDB, snapshot.Used, snapshot.Limit)
	return p.Run(ctx)
}

// Local filesystem/database access is the operator authority. This command is
// not exposed to the model or addressed chat. Stop/resume do not grant tokens.
func controlCmd(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("control: want status|stop|resume|extend|compact")
	}
	command := args[0]
	fs := flag.NewFlagSet("clyde control "+command, flag.ContinueOnError)
	path := fs.String("memory", filepath.Join(repoRoot(), "memory", "episodes.db"), "session database")
	extra := fs.Int64("requests", 0, "explicit additional requests (extend only)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("control: unexpected arguments")
	}
	if command != "extend" && *extra != 0 {
		return errors.New("control: --requests requires extend")
	}
	if _, err := os.Stat(*path); err != nil {
		return err
	}
	j, err := session.Open(*path)
	if err != nil {
		return err
	}
	defer j.Close()
	s, err := j.Load(ctx)
	if err != nil {
		return err
	}
	switch command {
	case "status":
	case "stop":
		err = j.Control(ctx, true, 0)
	case "resume":
		err = j.Control(ctx, false, 0)
	case "extend":
		if *extra <= 0 {
			return errors.New("control extend: positive --requests required")
		}
		err = j.Extend(ctx, *extra)
	case "compact":
		now := time.Now().UTC()
		err = j.Accept(ctx, []session.Input{{Key: "operator-compact:" + now.Format(time.RFC3339Nano), At: now, Kind: "operator_compact", Text: "Operator requested compaction through the production path."}})
	default:
		return errors.New("control: want status|stop|resume|extend|compact")
	}
	if err != nil {
		return err
	}
	s, err = j.Load(ctx)
	if err != nil {
		return err
	}
	account, err := j.Accounting(ctx)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"revision": s.Revision, "used": s.Used, "limit": s.Limit, "stopped": s.Stopped, "pause": s.State.Pause, "wait": s.State.Wait, "accounting": account})
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
