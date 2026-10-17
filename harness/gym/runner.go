package gym

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/lox/clyde/harness/goal"
	"github.com/lox/clyde/harness/rcon"
)

const (
	MCHost   = "127.0.0.1"
	MCPort   = "25565"
	RCONAddr = "127.0.0.1:25575"

	// DefaultRCONPassword matches gym/server.properties.
	DefaultRCONPassword = "clyde-gym"

	joinTimeout  = 60 * time.Second
	pollInterval = time.Second
	stopGrace    = 5 * time.Second
)

// RCONPassword returns GYM_RCON_PASSWORD or the local default.
func RCONPassword() string {
	if p := os.Getenv("GYM_RCON_PASSWORD"); p != "" {
		return p
	}
	return DefaultRCONPassword
}

// Runner runs tasks one after another against the gym server.
type Runner struct {
	Root     string        // repository root
	Agent    string        // agent command, run with bash -c
	Username string        // MC_USERNAME for the agent
	Timeout  time.Duration // overrides each task's timeout when non-zero
	RunDir   string        // gym/runs/<timestamp>
	Log      io.Writer     // progress output
}

// Result is the outcome of one task.
type Result struct {
	Task      string    `json:"task"`
	Result    string    `json:"result"` // pass, fail, or error
	Seconds   float64   `json:"seconds"`
	Timeout   Duration  `json:"timeout"`
	AgentExit string    `json:"agent_exit"`
	Detail    string    `json:"detail"`
	StartedAt time.Time `json:"started_at"`
}

// Run plays one task and always returns a result; setup problems are
// reported as result "error".
func (r *Runner) Run(ctx context.Context, t *Task) Result {
	timeout := time.Duration(t.Timeout)
	if r.Timeout > 0 {
		timeout = r.Timeout
	}
	res := Result{Task: t.Name, Timeout: Duration(timeout), StartedAt: time.Now().UTC(), AgentExit: "not started"}
	if err := r.run(ctx, t, timeout, &res); err != nil {
		res.Result = "error"
		res.Detail = err.Error()
	}
	return res
}

func (r *Runner) run(ctx context.Context, t *Task, timeout time.Duration, res *Result) error {
	dir := filepath.Join(r.RunDir, t.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	goalPath := filepath.Join(dir, "goal.json")
	readyPath := filepath.Join(dir, "ready")
	b, err := json.MarshalIndent(t.Goal, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(goalPath, b, 0o644); err != nil {
		return err
	}

	fmt.Fprintf(r.Log, "== %s: resetting world\n", t.Name)
	resetStart := time.Now()
	if err := r.paper(ctx, "reset"); err != nil {
		return err
	}
	if err := r.paper(ctx, "start"); err != nil {
		return err
	}
	fmt.Fprintf(r.Log, "== %s: server ready in %.1fs\n", t.Name, time.Since(resetStart).Seconds())

	conn, err := dialRCON(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	agent, err := r.startAgent(dir, goalPath, readyPath)
	if err != nil {
		return err
	}
	defer func() { res.AgentExit = agent.stop() }()

	if err := waitForPlayer(ctx, conn, r.Username, agent); err != nil {
		return err
	}
	for _, cmd := range t.Setup {
		cmd = strings.ReplaceAll(cmd, "{player}", r.Username)
		out, err := conn.Exec(cmd)
		if err != nil {
			return fmt.Errorf("setup %q: %w", cmd, err)
		}
		if CommandFailed(out) {
			return fmt.Errorf("setup %q: %s", cmd, out)
		}
	}
	obs := goal.RCON{Conn: conn}
	if got, err := goal.Evaluate(ctx, obs, r.Username, t.Goal.Until); err != nil {
		return fmt.Errorf("checking goal after setup: %w", err)
	} else if got.Satisfied {
		return fmt.Errorf("goal already holds after setup (%s); the task is broken", got.Detail)
	}
	if err := os.WriteFile(readyPath, nil, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(r.Log, "== %s: playing (timeout %s)\n", t.Name, timeout)

	start := time.Now()
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	for {
		got, err := goal.Evaluate(ctx, obs, r.Username, t.Goal.Until)
		res.Seconds = time.Since(start).Seconds()
		if err != nil {
			res.Detail = "check error: " + err.Error()
		} else {
			res.Detail = got.Detail
		}
		switch {
		case err == nil && got.Satisfied:
			res.Result = "pass"
			return nil
		case agent.exited():
			res.Result = "fail"
			res.Detail += "; agent exited"
			return nil
		case time.Since(start) >= timeout:
			res.Result = "fail"
			res.Detail += "; timed out"
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		case <-agent.done:
		}
	}
}

func (r *Runner) paper(ctx context.Context, verb string) error {
	cmd := exec.CommandContext(ctx, filepath.Join(r.Root, "gym", "bin", "paper"), verb)
	cmd.Stdout, cmd.Stderr = r.Log, r.Log
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gym/bin/paper %s: %w", verb, err)
	}
	return nil
}

func dialRCON(ctx context.Context) (*rcon.Client, error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		c, err := rcon.Dial(ctx, RCONAddr, RCONPassword())
		if err == nil || errors.Is(err, rcon.ErrAuth) || time.Now().After(deadline) {
			return c, err
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func waitForPlayer(ctx context.Context, conn *rcon.Client, player string, agent *agentProc) error {
	deadline := time.Now().Add(joinTimeout)
	for {
		out, err := conn.Exec("list")
		if err != nil {
			return err
		}
		if _, names, ok := strings.Cut(out, ": "); ok {
			for _, n := range strings.Split(names, ", ") {
				if n == player {
					return nil
				}
			}
		}
		if agent.exited() {
			return fmt.Errorf("agent exited before %s joined; see agent.log", player)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s did not join within %s; see agent.log", player, joinTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

type agentProc struct {
	cmd  *exec.Cmd
	done chan struct{}
	log  *os.File
}

// startAgent launches the agent in its own process group so stopping it
// also stops anything it spawned.
func (r *Runner) startAgent(dir, goalPath, readyPath string) (*agentProc, error) {
	log, err := os.Create(filepath.Join(dir, "agent.log"))
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("bash", "-c", r.Agent)
	cmd.Dir = r.Root
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = append(os.Environ(),
		"GYM_TASK="+goalPath,
		"GYM_READY="+readyPath,
		"MC_HOST="+MCHost,
		"MC_PORT="+MCPort,
		"MC_USERNAME="+r.Username,
		"GYM_RCON_ADDR="+RCONAddr,
		"GYM_RCON_PASSWORD="+RCONPassword(),
	)
	if err := cmd.Start(); err != nil {
		log.Close()
		return nil, fmt.Errorf("starting agent: %w", err)
	}
	a := &agentProc{cmd: cmd, done: make(chan struct{}), log: log}
	go func() { cmd.Wait(); close(a.done) }()
	return a, nil
}

func (a *agentProc) exited() bool {
	select {
	case <-a.done:
		return true
	default:
		return false
	}
}

// stop ends the agent and describes how it ended: "exit N" if it ended on
// its own, "stopped" if it ended when asked, "killed" if it had to be.
func (a *agentProc) stop() string {
	defer a.log.Close()
	pgid := -a.cmd.Process.Pid
	stopped := !a.exited()
	if stopped {
		syscall.Kill(pgid, syscall.SIGTERM)
		select {
		case <-a.done:
		case <-time.After(stopGrace):
			syscall.Kill(pgid, syscall.SIGKILL)
			<-a.done
			return "killed"
		}
	}
	// Reap anything the agent left behind in its group.
	syscall.Kill(pgid, syscall.SIGKILL)
	if stopped {
		return "stopped"
	}
	if code := a.cmd.ProcessState.ExitCode(); code >= 0 {
		return fmt.Sprintf("exit %d", code)
	}
	return a.cmd.ProcessState.String()
}
