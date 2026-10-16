// Command judge runs Clyde's tier-1 judge loop against a body or fixture
// cards, or evaluates a backend against labelled fixtures.
//
//	judge [run] --body http://127.0.0.1:7777
//	judge [run] --fixtures harness/judge/testdata/cards --backend replay --replay harness/judge/testdata/replay.jsonl
//	judge eval --backend jev
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/lox/clyde/harness/jev"
	"github.com/lox/clyde/harness/judge"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "judge:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cmd := "run"
	if len(args) > 0 && (args[0] == "run" || args[0] == "eval") {
		cmd, args = args[0], args[1:]
	}
	root := repoRoot()
	fs := flag.NewFlagSet("judge "+cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		backendName = fs.String("backend", "jev", "backend: jev or replay")
		replayPath  = fs.String("replay", filepath.Join(root, "harness/judge/testdata/replay.jsonl"), "JSONL judge log to replay answers from (replay backend)")
		questions   = fs.String("questions", filepath.Join(root, "prompts/judge.yaml"), "question set")
		configPath  = fs.String("config", "", "optional YAML overriding default thresholds, rate, and deadline")
		fixtures    = fs.String("fixtures", "", "directory of fixture state cards (*.json)")
	)
	var (
		bodyURL  *string
		logPath  *string
		rate     *float64
		deadline *time.Duration
		labels   *string
		reps     *int
		timeout  *time.Duration
	)
	if cmd == "run" {
		bodyURL = fs.String("body", "", "body base URL to poll GET /state, e.g. http://127.0.0.1:7777")
		logPath = fs.String("log", filepath.Join(root, "memory/judge.jsonl"), "JSONL tick log to append to")
		rate = fs.Float64("rate", 0, "ticks per second (default from config: 2)")
		deadline = fs.Duration("deadline", 0, "per-tick deadline (default from config: 800ms)")
	} else {
		labels = fs.String("labels", filepath.Join(root, "harness/judge/testdata/labels.yaml"), "fixture labels")
		reps = fs.Int("reps", 3, "times to run each fixture")
		timeout = fs.Duration("timeout", 5*time.Second, "per-call timeout (calls over the tick deadline are counted)")
		logPath = fs.String("log", "", "optional JSONL file to write one record per call (usable as a replay file)")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := judge.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	qs, err := judge.LoadQuestions(*questions)
	if err != nil {
		return err
	}

	if cmd == "eval" {
		if *fixtures == "" {
			*fixtures = filepath.Join(root, "harness/judge/testdata/cards")
		}
		if *backendName == "jev" && os.Getenv("TYPESAFE_API_KEY") == "" {
			fmt.Fprintln(stderr, "judge eval: skipped, TYPESAFE_API_KEY is not set")
			return nil
		}
		backend, err := newBackend(*backendName, *replayPath)
		if err != nil {
			return err
		}
		cases, err := judge.LoadCases(*fixtures, *labels)
		if err != nil {
			return err
		}
		var log io.Writer
		if *logPath != "" {
			f, err := openLog(*logPath)
			if err != nil {
				return err
			}
			defer f.Close()
			log = f
		}
		report, err := judge.Eval(ctx, backend, qs, cfg, cases, *reps, *timeout, log)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%d fixtures x %d reps\n", len(cases), *reps)
		report.Print(stdout)
		return nil
	}

	if *rate > 0 {
		cfg.Rate = *rate
	}
	if *deadline > 0 {
		cfg.Deadline = *deadline
	}
	var src judge.Source
	switch {
	case *bodyURL != "" && *fixtures != "":
		return errors.New("use one of --body or --fixtures")
	case *bodyURL != "":
		src = judge.BodySource{URL: *bodyURL}
	case *fixtures != "":
		fx, err := judge.LoadFixtures(*fixtures)
		if err != nil {
			return err
		}
		src = &judge.FixtureSource{Fixtures: fx}
	default:
		return errors.New("need --body or --fixtures")
	}
	backend, err := newBackend(*backendName, *replayPath)
	if err != nil {
		return err
	}
	log, err := openLog(*logPath)
	if err != nil {
		return err
	}
	defer log.Close()

	out := json.NewEncoder(stdout)
	j := judge.New(src, backend, qs, cfg)
	return j.Run(ctx, log, func(it judge.Interrupt) {
		if err := out.Encode(it); err != nil {
			fmt.Fprintln(stderr, "judge: write interrupt:", err)
		}
	})
}

func newBackend(name, replayPath string) (judge.Backend, error) {
	switch name {
	case "jev":
		c, err := jev.FromEnv()
		if err != nil {
			return nil, err
		}
		return judge.JevBackend{Client: c}, nil
	case "replay":
		return judge.LoadReplay(replayPath)
	}
	return nil, fmt.Errorf("unknown backend %q (want jev or replay)", name)
}

func openLog(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

// repoRoot finds the Clyde checkout by walking up from the working directory
// to the first directory containing prompts/judge.yaml, so defaults work from
// the repo root or from harness/. It falls back to ".".
func repoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "prompts", "judge.yaml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}
