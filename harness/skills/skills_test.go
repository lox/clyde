package skills_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lox/clyde/harness/body"
	"github.com/lox/clyde/harness/executor"
	"github.com/lox/clyde/harness/skills"
)

func candidate(t *testing.T, s skills.Store) {
	t.Helper()
	d := filepath.Join(s.Dir, "craft-item")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	for n, v := range map[string]string{"skill.md": "Craft an item with args.item", "index.js": "return args.item", "test.yaml": "name: test"} {
		if err := os.WriteFile(filepath.Join(d, n), []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAdmissionExecutionAndPromotion(t *testing.T) {
	s := skills.Store{Dir: t.TempDir()}
	candidate(t, s)
	c, err := s.LoadCandidate("craft-item")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load("craft-item"); err == nil {
		t.Fatal("unadmitted candidate exposed")
	}
	if hits, err := s.Find("craft"); err != nil || len(hits) != 0 {
		t.Fatalf("Find: %v %v", hits, err)
	}
	if err := s.RecordGate(c.Name, c.Digest, true, "independent goal passed"); err != nil {
		t.Fatal(err)
	}
	if hits, err := s.Find("craft item"); err != nil || len(hits) != 1 {
		t.Fatalf("Find: %v %v", hits, err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/eval" && r.URL.Path != "/action" {
			t.Errorf("path %s", r.URL.Path)
		}
		var req struct {
			Code      string
			TimeoutMs int
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if !strings.Contains(req.Code, `const args = {"item":"oak_planks"};`) || !strings.Contains(req.Code, "return args.item") || req.TimeoutMs != 3210 {
			t.Errorf("request: %+v", req)
		}
		w.Write([]byte(`{"id":1,"token":"skill-owner","type":"eval","status":"succeeded","elapsedMs":0,"startedAt":0,"params":{}}`))
	}))
	defer server.Close()
	if _, err := s.Run(context.Background(), &body.Client{URL: server.URL}, "craft-item", json.RawMessage(`{"item":"oak_planks"}`), 3210); err != nil {
		t.Fatal(err)
	}
	tb := &executor.Toolbox{Body: &body.Client{URL: server.URL}, SkillsDir: s.Dir}
	testedTools := 0
	for _, tool := range tb.Tools() {
		switch tool.Name {
		case "find_skill":
			testedTools++
			text, err := tool.Run(context.Background(), json.RawMessage(`{"query":"craft"}`))
			if err != nil || !strings.Contains(text, "args.item") {
				t.Fatalf("find_skill: %s %v", text, err)
			}
		case "run_skill":
			testedTools++
			text, err := tool.Run(context.Background(), json.RawMessage(`{"name":"craft-item","args":{"item":"oak_planks"},"timeoutMs":3210}`))
			if err != nil || !strings.Contains(text, `"status":"succeeded"`) {
				t.Fatalf("run_skill: %s %v", text, err)
			}
			for _, raw := range []string{`{"name":"missing","args":{}}`, `{"name":"../craft-item","args":{}}`, `{"name":"craft-item","args":[]}`} {
				if _, err := tool.Run(context.Background(), json.RawMessage(raw)); err == nil {
					t.Errorf("accepted %s", raw)
				}
			}
		}
	}
	if testedTools != 2 {
		t.Fatalf("expected both skill tools, tested %d", testedTools)
	}
	tb.SkillsDir = ""
	for _, tool := range tb.Tools() {
		if tool.Name == "find_skill" || tool.Name == "run_skill" {
			t.Fatalf("disabled skills still expose %s", tool.Name)
		}
	}
	active := skills.Store{Dir: t.TempDir()}
	if err := active.Promote(s, "craft-item"); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"skill.md", "index.js", "test.yaml"} {
		path := filepath.Join(s.Dir, "craft-item", file)
		original, _ := os.ReadFile(path)
		os.WriteFile(path, append(original, '\n'), 0o644)
		if _, err := s.Load("craft-item"); err == nil {
			t.Errorf("changed %s admitted", file)
		}
		if err := active.Promote(s, "craft-item"); err == nil {
			t.Errorf("changed %s promoted", file)
		}
		if _, err := active.Load("craft-item"); err != nil {
			t.Errorf("failed promotion damaged active: %v", err)
		}
		os.WriteFile(path, original, 0o644)
	}
	if err := s.RecordGate(c.Name, c.Digest, false, "goal failed"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(c.Name); err == nil {
		t.Fatal("failed gate retained admission")
	}
	if _, err := active.Load(c.Name); err != nil {
		t.Fatal(err)
	}
	if err := active.Promote(s, c.Name); err == nil {
		t.Fatal("failed candidate promoted")
	}
	if got, err := active.Load(c.Name); err != nil || got.Digest != c.Digest {
		t.Fatalf("failed gate damaged live: %+v %v", got, err)
	}
	for _, name := range []string{"../craft-item", "a/b", ".", "", "Craft", "a\\b"} {
		if _, err := s.LoadCandidate(name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	if err := os.Symlink(filepath.Join(active.Dir, c.Name), filepath.Join(s.Dir, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadCandidate("escape"); err == nil {
		t.Fatal("accepted directory symlink")
	}
	if err := s.RecordGate("escape", c.Digest, false, ""); err == nil {
		t.Fatal("gate followed directory symlink")
	}
	if _, err := active.Load(c.Name); err != nil {
		t.Fatalf("symlink gate damaged target: %v", err)
	}
}
