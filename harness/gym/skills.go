package gym

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lox/clyde/harness/skills"
)

// CandidateRun is the frozen candidate and invocation given only to the gate agent.
type CandidateRun struct {
	Skill      skills.Skill
	Invocation Invocation
}

// Gate tests one unadmitted snapshot in a reset gym world and records admission
// only from the runner's independent goal result. Use a separate Store for a
// proposed revision so failure cannot revoke a working live installation.
// Runner must use gym/agents/executor; this function supplies its candidate mode.
func Gate(ctx context.Context, store skills.Store, name string, runner Runner) (Result, error) {
	if err := store.RecordGate(name, "", false, ""); err != nil {
		return Result{}, err
	}
	k, err := store.LoadCandidate(name)
	if err != nil {
		return Result{}, err
	}
	task, err := ParseTask("test.yaml", k.Test)
	if err != nil {
		return Result{}, err
	}
	if task.Skill == nil {
		return Result{}, fmt.Errorf("skill test requires skill.args")
	}
	if err := os.MkdirAll(runner.RunDir, 0o755); err != nil {
		return Result{}, err
	}
	snapshot, err := os.CreateTemp(runner.RunDir, "candidate-*.json")
	if err != nil {
		return Result{}, err
	}
	defer os.Remove(snapshot.Name())
	err = json.NewEncoder(snapshot).Encode(CandidateRun{k, *task.Skill})
	closeErr := snapshot.Close()
	if err != nil {
		return Result{}, err
	}
	if closeErr != nil {
		return Result{}, closeErr
	}
	abs, err := filepath.Abs(snapshot.Name())
	if err != nil {
		return Result{}, err
	}
	runner.Env = append(runner.Env, "GYM_SKILL_SNAPSHOT="+abs)
	res := runner.Run(ctx, task, 0)
	if err := store.RecordGate(name, k.Digest, res.Result == "pass", filepath.Join(runner.RunDir, task.Name)+": "+res.Detail); err != nil {
		return res, err
	}
	return res, nil
}
