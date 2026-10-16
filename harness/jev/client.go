// Package jev is a small client for TypeSafe's System One API
// (POST /v1/systemone). See https://docs.typesafe.ai/api.md.
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
)

const (
	// DefaultURL is the System One evaluation endpoint.
	DefaultURL = "https://api.typesafe.ai/v1/systemone"
	// DefaultModel tracks TypeSafe's newest Jev release.
	DefaultModel = "jev-latest"
)

// Question is one typed question. Type is "noul", "choice", or "score";
// Instructions and Criteria take the shapes the API documents for each type.
type Question struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

// Answer is one typed answer. Which fields are set depends on Type.
type Answer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
}

// Usage is the token usage of one request. Jev bills input tokens only.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Request is the body of POST /v1/systemone.
type Request struct {
	Model     string              `json:"model"`
	State     json.RawMessage     `json:"state"`
	Questions map[string]Question `json:"questions"`
}

// Response is the decoded body of a successful evaluation.
type Response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

// StatusError is returned for non-2xx responses. Body is a bounded excerpt of
// the response body; the request (and so the API key) is never included.
type StatusError struct {
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("jev: status %d: %s", e.StatusCode, e.Body)
}

// Client calls the System One API. The zero value is not usable; use New.
type Client struct {
	URL   string
	Model string
	HTTP  *http.Client
	key   string
}

// New returns a client for the given API key with default URL and model.
func New(apiKey string) *Client {
	return &Client{URL: DefaultURL, Model: DefaultModel, HTTP: http.DefaultClient, key: apiKey}
}

// FromEnv returns a client using TYPESAFE_API_KEY, or an error if it is unset.
func FromEnv() (*Client, error) {
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		return nil, errors.New("jev: TYPESAFE_API_KEY is not set")
	}
	return New(key), nil
}

// Evaluate asks every question about state in one request. state must be a
// JSON value (Jev accepts a string, object, or array). Deadlines come from ctx.
func (c *Client) Evaluate(ctx context.Context, state json.RawMessage, questions map[string]Question) (*Response, error) {
	body, err := json.Marshal(Request{Model: c.Model, State: state, Questions: questions})
	if err != nil {
		return nil, fmt.Errorf("jev: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("jev: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		// net/http errors carry the method and URL, never headers.
		return nil, fmt.Errorf("jev: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, &StatusError{StatusCode: resp.StatusCode, Body: string(bytes.TrimSpace(excerpt))}
	}
	var out Response
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("jev: decode response: %w", err)
	}
	return &out, nil
}
