package jev

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testKey = "sk-test-secret-value"

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(testKey)
	c.URL = srv.URL
	return c
}

func TestEvaluateRequestShapeAndAnswers(t *testing.T) {
	var gotAuth, gotType string
	var got map[string]json.RawMessage
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("Content-Type")
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("request is not JSON: %v", err)
		}
		io.WriteString(w, `{
			"model": "jev-1.13.0",
			"answers": {
				"danger": {"type": "noul", "noul": 0.85},
				"interrupt": {"type": "choice", "choice": "danger", "confidence": 0.96,
					"probabilities": {"none": 0.02, "danger": 0.97, "stuck": 0.01}},
				"progress": {"type": "score", "score": 1.05, "confidence": 0.92,
					"legend": {"0": "none", "1": "some", "2": "steady"},
					"probabilities": {"0": 0.0, "1": 0.95, "2": 0.05}}
			},
			"usage": {"input_tokens": 552, "output_tokens": 77}
		}`)
	})

	state := json.RawMessage(`{"bot":{"health":20}}`)
	questions := map[string]Question{
		"danger":    {Type: "noul", Instructions: "In danger?"},
		"interrupt": {Type: "choice", Instructions: "Why stop?", Criteria: map[string]any{"none": nil, "danger": "hurt"}},
		"progress":  {Type: "score", Instructions: "Progress?", Criteria: []string{"none", "some", "steady"}},
	}
	resp, err := c.Evaluate(context.Background(), state, questions)
	if err != nil {
		t.Fatal(err)
	}

	if gotAuth != "Bearer "+testKey {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotType != "application/json" {
		t.Errorf("Content-Type = %q", gotType)
	}
	if string(got["model"]) != `"jev-latest"` {
		t.Errorf("model = %s", got["model"])
	}
	if string(got["state"]) != string(state) {
		t.Errorf("state = %s, want the card passed through unchanged", got["state"])
	}
	var qs map[string]map[string]any
	if err := json.Unmarshal(got["questions"], &qs); err != nil {
		t.Fatal(err)
	}
	if qs["danger"]["type"] != "noul" || qs["interrupt"]["type"] != "choice" || qs["progress"]["type"] != "score" {
		t.Errorf("questions = %v", qs)
	}
	if _, ok := qs["danger"]["criteria"]; ok {
		t.Errorf("noul without criteria should omit the field: %v", qs["danger"])
	}

	if a := resp.Answers["danger"]; a.Type != "noul" || a.Noul == nil || *a.Noul != 0.85 {
		t.Errorf("noul answer = %+v", a)
	}
	if a := resp.Answers["interrupt"]; a.Choice != "danger" || *a.Confidence != 0.96 || a.Probabilities["danger"] != 0.97 {
		t.Errorf("choice answer = %+v", a)
	}
	if a := resp.Answers["progress"]; *a.Score != 1.05 || *a.Confidence != 0.92 || a.Legend["2"] != "steady" || a.Probabilities["1"] != 0.95 {
		t.Errorf("score answer = %+v", a)
	}
	if resp.Usage != (Usage{InputTokens: 552, OutputTokens: 77}) || resp.Model != "jev-1.13.0" {
		t.Errorf("usage/model = %+v %q", resp.Usage, resp.Model)
	}
}

func TestEvaluateStatusErrorOmitsKey(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":"invalid api key"}`)
	})
	_, err := c.Evaluate(context.Background(), json.RawMessage(`{}`), map[string]Question{})
	var se *StatusError
	if !errors.As(err, &se) || se.StatusCode != 401 || !strings.Contains(se.Body, "invalid api key") {
		t.Fatalf("err = %v, want StatusError 401", err)
	}
	if strings.Contains(err.Error(), testKey) {
		t.Fatalf("error leaks the key: %v", err)
	}
}

func TestEvaluateContextTimeout(t *testing.T) {
	release := make(chan struct{})
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Evaluate(ctx, json.RawMessage(`{}`), map[string]Question{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("Evaluate ignored the deadline")
	}
	if strings.Contains(err.Error(), testKey) {
		t.Fatalf("error leaks the key: %v", err)
	}
}
