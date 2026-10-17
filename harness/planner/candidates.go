package planner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lox/clyde/harness/gym"
	"github.com/lox/clyde/harness/skills"
)

// Candidates stages immutable proposals outside the admitted library. Loading,
// gating, receipts and promotion remain exclusively owned by slice 5's Store.
type Candidates struct{ dir string }

// NewCandidates rejects overlapping candidate/live roots, including symlink aliases.
func NewCandidates(dir, liveDir string) (*Candidates, error) {
	if dir == "" || liveDir == "" {
		return nil, errors.New("candidate and live skill roots required")
	}
	for _, path := range []string{dir, liveDir} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return nil, err
		}
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	live, err := filepath.EvalSymlinks(liveDir)
	if err != nil {
		return nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	live, err = filepath.Abs(live)
	if err != nil {
		return nil, err
	}
	for _, pair := range [][2]string{{root, live}, {live, root}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err != nil {
			return nil, err
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return nil, errors.New("candidate root must be separate from live skills")
		}
	}
	return &Candidates{dir: root}, nil
}

// Save writes one fresh proposal root and validates it with the real skill/task
// loaders. Revisions get distinct roots, so play cannot overwrite prior proposals
// or admissions. It never invokes Gate, RecordGate, Promote, or an eval action.
func (s *Candidates) Save(ctx context.Context, c Candidate) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !filepath.IsLocal(c.Name) || filepath.Base(c.Name) != c.Name || c.Name == "." || len(c.Name) > 80 {
		return "", errors.New("invalid candidate name")
	}
	if len(c.Description)+len(c.Code)+len(c.Test) > 32768 {
		return "", errors.New("candidate exceeds 32 KiB")
	}
	root, err := os.MkdirTemp(s.dir, "proposal-")
	if err != nil {
		return "", err
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(root)
		}
	}()
	dir := filepath.Join(root, c.Name)
	if err := os.Mkdir(dir, 0o700); err != nil {
		return "", err
	}
	for file, data := range map[string]string{"skill.md": c.Description, "index.js": c.Code, "test.yaml": c.Test} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(data), 0o600); err != nil {
			return "", err
		}
	}
	k, err := (skills.Store{Dir: root}).LoadCandidate(c.Name)
	if err != nil {
		return "", err
	}
	task, err := gym.ParseTask("test.yaml", k.Test)
	if err != nil {
		return "", err
	}
	if task.Skill == nil {
		return "", fmt.Errorf("candidate test requires skill.args")
	}
	keep = true
	return dir, nil
}
