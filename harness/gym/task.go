// Package gym runs gym tasks: it resets a throwaway Paper server, sets up a
// scenario over RCON, lets an agent play, and scores the task's goal.
package gym

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/lox/clyde/harness/goal"
)

// Templates lists the world templates gym/bin/paper can restore.
var Templates = []string{"mcbot"}

// Task is one gym/tasks/*.yaml file; see gym/tasks/README.md.
type Task struct {
	Name     string      `json:"name"`
	Template string      `json:"template"`
	Setup    []string    `json:"setup"`
	Goal     goal.Goal   `json:"goal"`
	Timeout  Duration    `json:"timeout"`
	Settle   Duration    `json:"settle,omitempty"` // wait for asynchronous setup effects before readiness
	Skill    *Invocation `json:"skill,omitempty"`  // deterministic candidate gate only
}

// Invocation specifies the candidate arguments, not the model's tool sequence.
type Invocation struct {
	Args      json.RawMessage `json:"args"`
	TimeoutMs int             `json:"timeoutMs,omitempty"`
}

// Duration is a time.Duration written as a Go duration string such as "90s".
type Duration time.Duration

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("duration must be a string such as \"90s\": %w", err)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(time.Duration(d).String()) }

// LoadTask reads and validates a task file.
func LoadTask(path string) (*Task, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseTask(path, b)
}

// ParseTask validates a captured task snapshot. path supplies its expected name.
func ParseTask(path string, b []byte) (*Task, error) {
	// Decode YAML 1.2 (where a key named y stays a string), then go through
	// JSON so the schema-generated validation runs.
	var raw any
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	j, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(j))
	dec.DisallowUnknownFields()
	var t Task
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := t.validate(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &t, nil
}

func (t *Task) validate(file string) error {
	if t.Name != file {
		return fmt.Errorf("name %q must match the file name %q", t.Name, file)
	}
	known := false
	for _, tpl := range Templates {
		known = known || t.Template == tpl
	}
	if !known {
		return fmt.Errorf("unknown template %q (have %v)", t.Template, Templates)
	}
	if t.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	if t.Settle < 0 || time.Duration(t.Settle) > 10*time.Second {
		return fmt.Errorf("settle must be between 0 and 10s")
	}
	if t.Skill != nil {
		var args map[string]json.RawMessage
		if json.Unmarshal(t.Skill.Args, &args) != nil || args == nil {
			return fmt.Errorf("skill.args must be an object")
		}
		if t.Skill.TimeoutMs < 0 || t.Skill.TimeoutMs > 600000 {
			return fmt.Errorf("invalid skill timeoutMs")
		}
	}
	kind, err := goal.Kind(t.Goal.Until)
	if err != nil {
		return err
	}
	if kind == "judged" {
		return goal.ErrJudged
	}
	return nil
}
