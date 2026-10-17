package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lox/clyde/harness/body"
	"github.com/lox/clyde/harness/gym"
	"github.com/lox/clyde/harness/skills"
)

func skillCmd(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: clyde skill gate|promote [--dir skills] [--from candidates] name...")
	}
	verb := args[0]
	fs := flag.NewFlagSet("clyde skill "+verb, flag.ContinueOnError)
	dir := fs.String("dir", filepath.Join(repoRoot(), "skills"), "library to gate or promote into")
	from := fs.String("from", "", "separate already gated candidate library (promote only)")
	snapshot := fs.String("snapshot", "", "frozen candidate run file (internal gym agent)")
	bodyURL := fs.String("body", "http://127.0.0.1:7787", "candidate agent body URL")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if verb == "candidate" {
		b, err := os.ReadFile(*snapshot)
		if err != nil {
			return err
		}
		var run gym.CandidateRun
		if err := json.Unmarshal(b, &run); err != nil {
			return err
		}
		client := &body.Client{URL: *bodyURL}
		defer func() {
			stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			client.Stop(stop)
		}()
		status, err := run.Skill.Run(ctx, client, run.Invocation.Args, run.Invocation.TimeoutMs)
		if err != nil {
			return err
		}
		if err := json.NewEncoder(os.Stdout).Encode(status); err != nil {
			return err
		}
		if status.Status != "succeeded" {
			detail := ""
			if status.Error != nil {
				detail = *status.Error
			}
			return fmt.Errorf("candidate %s: %s", status.Status, detail)
		}
		return nil
	}
	if verb != "gate" && verb != "promote" {
		return fmt.Errorf("unknown skill command %q", verb)
	}
	if fs.NArg() == 0 {
		return errors.New("at least one skill name is required")
	}
	store := skills.Store{Dir: *dir}
	var results []gym.Result
	var failures []error
	for _, name := range fs.Args() {
		if verb == "promote" {
			if *from == "" {
				return errors.New("promote requires --from candidate-root")
			}
			if err := store.Promote(skills.Store{Dir: *from}, name); err != nil {
				return err
			}
			fmt.Println("promoted", name)
			continue
		}
		runner := gym.Runner{Root: repoRoot(), Agent: filepath.Join(repoRoot(), "gym", "agents", "executor"), Username: "clyde", RunDir: filepath.Join(repoRoot(), "gym", "runs", time.Now().UTC().Format("20060102T150405.000000000Z")+"-"+name), Log: os.Stderr}
		res, err := gym.Gate(ctx, store, name, runner)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		res.Task = name
		results = append(results, res)
		b, _ := json.MarshalIndent(res, "", "  ")
		if err := os.WriteFile(filepath.Join(runner.RunDir, "result.json"), b, 0o644); err != nil {
			return err
		}
		if res.Result != "pass" {
			failures = append(failures, fmt.Errorf("%s gate %s: %s", name, res.Result, res.Detail))
		}
	}
	if verb == "gate" {
		fmt.Print(gym.Scoreboard("skill gates (no model)", results))
	}
	return errors.Join(failures...)
}
