package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BodySource polls the body's GET /state.
type BodySource struct {
	URL  string // base URL, e.g. http://127.0.0.1:7777
	HTTP *http.Client
}

func (s BodySource) Card(ctx context.Context) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.URL, "/")+"/state", nil)
	if err != nil {
		return nil, err
	}
	client := s.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /state: status %d", resp.StatusCode)
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("GET /state: response is not JSON")
	}
	return body, nil
}

// Fixture is a named state card read from disk.
type Fixture struct {
	Name string
	Card json.RawMessage
}

// LoadFixtures reads every *.json card in dir, sorted by file name. The name
// is the file name without its extension.
func LoadFixtures(dir string) ([]Fixture, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no *.json state cards in %s", dir)
	}
	sort.Strings(paths)
	out := make([]Fixture, 0, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		if !json.Valid(data) {
			return nil, fmt.Errorf("%s: not valid JSON", p)
		}
		out = append(out, Fixture{Name: strings.TrimSuffix(filepath.Base(p), ".json"), Card: data})
	}
	return out, nil
}

// FixtureSource yields fixture cards once each, in order, then io.EOF.
type FixtureSource struct {
	Fixtures []Fixture
	next     int
}

func (s *FixtureSource) Card(ctx context.Context) (json.RawMessage, error) {
	if s.next >= len(s.Fixtures) {
		return nil, io.EOF
	}
	s.next++
	return s.Fixtures[s.next-1].Card, nil
}
