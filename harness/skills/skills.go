// Package skills loads trusted JavaScript skills and content-bound gym admissions.
// Receipts are a local development gate, not a security boundary.
package skills

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/lox/clyde/harness/body"
)

// Store is a library root. Candidate stores use the same layout outside the live root.
type Store struct{ Dir string }

// Skill is an immutable snapshot of the three source files.
type Skill struct {
	Name        string
	Description string
	Code        string
	Test        []byte
	Digest      string
}

var validName = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

const receiptFile = ".admission.json"

type receipt struct {
	Digest   string    `json:"sha256"`
	PassedAt time.Time `json:"passed_at"`
	Evidence string    `json:"evidence"`
}

// LoadCandidate reads a skill without requiring admission. Only gate agents use this bypass.
func (s Store) LoadCandidate(name string) (Skill, error) {
	if !validName.MatchString(name) {
		return Skill{}, fmt.Errorf("invalid skill name %q", name)
	}
	k := Skill{Name: name}
	dir, err := os.Lstat(filepath.Join(s.Dir, name))
	if err != nil {
		return Skill{}, err
	}
	if !dir.IsDir() {
		return Skill{}, fmt.Errorf("skill %s must be a real directory, not a symlink", name)
	}
	h := sha256.New()
	for _, file := range []string{"skill.md", "index.js", "test.yaml"} {
		p := filepath.Join(s.Dir, name, file)
		info, err := os.Lstat(p)
		if err != nil {
			return Skill{}, err
		}
		if !info.Mode().IsRegular() {
			return Skill{}, fmt.Errorf("skill %s: %s must be a regular file", name, file)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return Skill{}, fmt.Errorf("skill %s: %w", name, err)
		}
		if len(strings.TrimSpace(string(b))) == 0 {
			return Skill{}, fmt.Errorf("skill %s: empty %s", name, file)
		}
		fmt.Fprintf(h, "%s\x00%d\x00", file, len(b))
		h.Write(b)
		switch file {
		case "skill.md":
			k.Description = string(b)
		case "index.js":
			k.Code = string(b)
		case "test.yaml":
			k.Test = b
		}
	}
	k.Digest = fmt.Sprintf("%x", h.Sum(nil))
	return k, nil
}

// Load refuses absent, stale, or failed admissions; it always reads current contents.
func (s Store) Load(name string) (Skill, error) {
	k, err := s.LoadCandidate(name)
	if err != nil {
		return Skill{}, err
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, name, receiptFile))
	var r receipt
	if err != nil || json.Unmarshal(b, &r) != nil || r.Digest != k.Digest || r.PassedAt.IsZero() {
		return Skill{}, fmt.Errorf("skill %s is not admitted for its current contents; run the gym gate", name)
	}
	return k, nil
}

// Find returns admitted matches, ranked by description word hits (at most five).
func (s Store) Find(query string) ([]Skill, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	type hit struct {
		k     Skill
		score int
	}
	var hits []hit
	for _, e := range entries {
		if !e.IsDir() || !validName.MatchString(e.Name()) {
			continue
		}
		k, err := s.Load(e.Name())
		if err != nil {
			continue
		} // incomplete and unadmitted candidates are invisible
		score := 0
		for _, word := range strings.Fields(strings.ToLower(query)) {
			score += strings.Count(strings.ToLower(k.Name+" "+k.Description), word)
		}
		if score > 0 {
			hits = append(hits, hit{k, score})
		}
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return b.score - a.score })
	var out []Skill
	for _, h := range hits[:min(5, len(hits))] {
		out = append(out, h.k)
	}
	return out, nil
}

// RecordGate revokes a previous receipt, and issues a new one only for a passing,
// unchanged snapshot. Callers must use independent gym results, never model claims.
func (s Store) RecordGate(name, digest string, passed bool, evidence string) error {
	k, err := s.LoadCandidate(name)
	if err != nil {
		return err
	}
	p := filepath.Join(s.Dir, name, receiptFile)
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !passed {
		return nil
	}
	if k.Digest != digest {
		return errors.New("skill changed during gating; not admitted")
	}
	b, err := json.MarshalIndent(receipt{digest, time.Now().UTC(), evidence}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// Run loads an admitted snapshot and invokes the existing body eval action.
func (s Store) Run(ctx context.Context, client *body.Client, name string, args json.RawMessage, timeoutMs int) (body.ActionStatus, error) {
	k, err := s.Load(name)
	if err != nil {
		return body.ActionStatus{}, err
	}
	return k.Run(ctx, client, args, timeoutMs)
}

// Run executes this snapshot. Gate agents use it for an unadmitted candidate.
// Cancellation also stops the body action: closing an HTTP request alone does not.
func (k Skill) Run(ctx context.Context, client *body.Client, args json.RawMessage, timeoutMs int) (body.ActionStatus, error) {
	req, err := k.Action(args, timeoutMs)
	if err != nil {
		return body.ActionStatus{}, err
	}
	status, err := client.Eval(ctx, req.Code, timeoutMs)
	if ctx.Err() != nil {
		stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if stopErr := client.Stop(stop); stopErr != nil {
			return status, errors.Join(err, stopErr)
		}
	}
	return status, err
}

// Action prepares the same validated snapshot for asynchronous executor dispatch.
func (k Skill) Action(args json.RawMessage, timeoutMs int) (body.EvalAction, error) {
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(args, &object); err != nil || object == nil {
		return body.EvalAction{}, errors.New("skill args must be a JSON object")
	}
	if timeoutMs < 0 || timeoutMs > 600_000 {
		return body.EvalAction{}, errors.New("timeoutMs must be between 0 (default) and 600000")
	}
	req := body.EvalAction{Type: "eval", Code: "const args = " + string(args) + ";\n" + k.Code}
	if timeoutMs > 0 {
		req.TimeoutMs = timeoutMs
	}
	return req, nil
}

// Promote publishes an already gated candidate snapshot, including its receipt.
// Failed/stale candidates never touch the live installation. Callers serialize
// promotions of the same name; this is a local library, not a multi-writer registry.
func (s Store) Promote(candidate Store, name string) error {
	k, err := candidate.Load(name)
	if err != nil {
		return err
	}
	r, err := os.ReadFile(filepath.Join(candidate.Dir, name, receiptFile))
	if err != nil {
		return err
	}
	var admission receipt
	if err := json.Unmarshal(r, &admission); err != nil || admission.Digest != k.Digest {
		return errors.New("candidate receipt changed during promotion")
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(s.Dir, ".promote-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for file, data := range map[string][]byte{"skill.md": []byte(k.Description), "index.js": []byte(k.Code), "test.yaml": k.Test, receiptFile: r} {
		if err := os.WriteFile(filepath.Join(tmp, file), data, 0o644); err != nil {
			return err
		}
	}
	dest := filepath.Join(s.Dir, name)
	// Preserve the old directory until the complete replacement is installed.
	backup, err := os.MkdirTemp(s.Dir, ".previous-")
	if err != nil {
		return err
	}
	keepBackup := false
	defer func() {
		if !keepBackup {
			os.RemoveAll(backup)
		}
	}()
	old := filepath.Join(backup, name)
	if err := os.Rename(dest, old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		if _, statErr := os.Stat(old); statErr == nil {
			if restoreErr := os.Rename(old, dest); restoreErr != nil {
				keepBackup = true
				return fmt.Errorf("%w; previous skill retained at %s", errors.Join(err, restoreErr), old)
			}
		}
		return err
	}
	return nil
}
