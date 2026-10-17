package gym

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lox/clyde/harness/goal"
)

func TestShippedTasksLoad(t *testing.T) {
	paths, err := filepath.Glob("../../gym/tasks/*.yaml")
	if err != nil || len(paths) < 4 {
		t.Fatalf("want at least 4 tasks, got %d (%v)", len(paths), err)
	}
	kinds := map[string]bool{}
	for _, p := range paths {
		task, err := LoadTask(p)
		if err != nil {
			t.Fatal(err)
		}
		k, _ := goal.Kind(task.Goal.Until)
		kinds[k] = true
	}
	for _, k := range []string{"items", "arrived", "built", "delivered"} {
		if !kinds[k] {
			t.Errorf("no task uses the %s predicate", k)
		}
	}
}

func TestLoadTask(t *testing.T) {
	const valid = `name: t
template: mcbot
timeout: 30s
setup: [time set day]
goal:
  description: go there
  until:
    arrived: {at: {x: 1, y: 64, z: -2.5}, radius: 2}
`
	tests := []struct {
		name, yaml, wantErr string
	}{
		{"valid", valid, ""},
		{"name mismatch", strings.Replace(valid, "name: t", "name: other", 1), "must match the file name"},
		{"unknown template", strings.Replace(valid, "mcbot", "flat", 1), "unknown template"},
		{"unknown field", valid + "extra: 1\n", "unknown field"},
		{"bad timeout", strings.Replace(valid, "30s", "30", 1), "duration"},
		{"judged rejected", strings.Replace(valid, "arrived: {at: {x: 1, y: 64, z: -2.5}, radius: 2}", "judged: {question: nice?}", 1), "judged"},
		{"two predicates", strings.Replace(valid, "radius: 2}", "radius: 2}\n    items: {item: minecraft:dirt, count: 1}", 1), "exactly one"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "t.yaml")
			if err := os.WriteFile(p, []byte(tt.yaml), 0o644); err != nil {
				t.Fatal(err)
			}
			task, err := LoadTask(p)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				// YAML 1.1 would read the key y as true and drop it.
				if got := task.Goal.Until.Arrived.At; got != (goal.Vec3{X: 1, Y: 64, Z: -2.5}) {
					t.Fatalf("at = %+v", got)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
