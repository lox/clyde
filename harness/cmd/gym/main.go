// Command gym runs gym tasks and prints a scoreboard. gym/bin/run wraps it.
//
//	gym [--agent cmd] [--timeout d] <task>... | --all
//	gym cheat    (used by gym/agents/cheat)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/lox/clyde/harness/goal"
	"github.com/lox/clyde/harness/gym"
	"github.com/lox/clyde/harness/rcon"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var err error
	if len(os.Args) > 1 && os.Args[1] == "cheat" {
		err = cheat(ctx)
	} else {
		err = run(ctx, os.Args[1:])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gym:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("gym", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	agent := fs.String("agent", "idle", "agent: a name in gym/agents/ or a shell command")
	all := fs.Bool("all", false, "run every task in gym/tasks/")
	timeout := fs.Duration("timeout", 0, "override every task's timeout")
	username := fs.String("username", "clyde", "MC_USERNAME for the agent")
	runs := fs.Int("runs", 1, "run each task this many times")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: gym/bin/run <task>...|--all [--agent <cmd>] [--timeout <duration>] [--runs <n>]")
		fs.PrintDefaults()
	}
	// Allow flags before and after task names.
	var names []string
	for {
		fs.Parse(args)
		if fs.NArg() == 0 {
			break
		}
		names = append(names, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if *all == (len(names) > 0) {
		fs.Usage()
		os.Exit(2)
	}

	tasksDir := filepath.Join(*root, "gym", "tasks")
	if *all {
		paths, err := filepath.Glob(filepath.Join(tasksDir, "*.yaml"))
		if err != nil {
			return err
		}
		sort.Strings(paths)
		for _, p := range paths {
			names = append(names, strings.TrimSuffix(filepath.Base(p), ".yaml"))
		}
	}
	var tasks []*gym.Task
	for _, n := range names {
		t, err := gym.LoadTask(filepath.Join(tasksDir, n+".yaml"))
		if err != nil {
			return err
		}
		tasks = append(tasks, t)
	}

	agentCmd := *agent
	if builtin := filepath.Join(*root, "gym", "agents", *agent); !strings.ContainsAny(*agent, " /") {
		if fi, err := os.Stat(builtin); err == nil && !fi.IsDir() {
			agentCmd = builtin
		}
	}

	runDir := filepath.Join(*root, "gym", "runs", time.Now().UTC().Format("20060102T150405Z"))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	r := &gym.Runner{Root: *root, Agent: agentCmd, Username: *username, Timeout: *timeout, RunDir: runDir, Log: os.Stderr}

	var results []gym.Result
tasks:
	for _, t := range tasks {
		for i := 1; i <= *runs; i++ {
			rep := i
			if *runs == 1 {
				rep = 0 // a single run keeps the plain task directory name
			}
			res := r.Run(ctx, t, rep)
			fmt.Fprintf(os.Stderr, "== %s: %s (%s)\n", t.Name, res.Result, res.Detail)
			results = append(results, res)
			if ctx.Err() != nil {
				break tasks
			}
		}
	}

	board := gym.Scoreboard(*agent, results)
	fmt.Print(board)
	fmt.Fprintf(os.Stderr, "\nrun written to %s\n", runDir)
	b, _ := json.MarshalIndent(map[string]any{"agent": *agent, "results": results}, "", "  ")
	if err := os.WriteFile(filepath.Join(runDir, "results.json"), b, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(runDir, "scoreboard.md"), []byte(board), 0o644); err != nil {
		return err
	}
	for _, res := range results {
		if res.Result != "pass" {
			os.Exit(1)
		}
	}
	return nil
}

// cheat waits for the runner's ready signal, then satisfies the goal over
// RCON. It proves each checker can pass without the body.
func cheat(ctx context.Context) error {
	ready, task, player := os.Getenv("GYM_READY"), os.Getenv("GYM_TASK"), os.Getenv("MC_USERNAME")
	if ready == "" || task == "" || player == "" {
		return fmt.Errorf("cheat: GYM_READY, GYM_TASK, and MC_USERNAME must be set")
	}
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	b, err := os.ReadFile(task)
	if err != nil {
		return err
	}
	var g goal.Goal
	if err := json.Unmarshal(b, &g); err != nil {
		return err
	}
	cmds, err := gym.CheatCommands(player, g)
	if err != nil {
		return err
	}
	addr := os.Getenv("GYM_RCON_ADDR")
	if addr == "" {
		addr = gym.RCONAddr
	}
	conn, err := rcon.Dial(ctx, addr, gym.RCONPassword())
	if err != nil {
		return err
	}
	defer conn.Close()
	for _, c := range cmds {
		out, err := conn.Exec(c)
		if err != nil {
			return err
		}
		fmt.Printf("cheat: %s -> %s\n", c, out)
		if gym.CommandFailed(out) {
			return fmt.Errorf("cheat: %q failed: %s", c, out)
		}
	}
	return nil
}
