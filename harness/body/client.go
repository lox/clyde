package body

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Client talks to one body over HTTP.
type Client struct {
	URL  string // base URL, e.g. http://127.0.0.1:7777
	HTTP *http.Client
}

// APIError is a non-200 answer from the body. Message is the body's error
// text, which is written for models (for example a schema violation).
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("body: %s (HTTP %d)", e.Message, e.Status) }

// Event is one entry of GET /events.
type Event struct {
	Seq  int            `json:"seq"`
	TS   int64          `json:"ts"`
	Type string         `json:"type"`
	Text string         `json:"text"`
	Data map[string]any `json:"data,omitempty"`
}

// EventPage is the answer to GET /events?since=N.
type EventPage struct {
	Events    []Event `json:"events"`
	LatestSeq int     `json:"latestSeq"`
	Missed    int     `json:"missed"`
}

// QueryResult is the answer to POST /query: eval code that ran beside the
// current action. Status is "succeeded" or "failed".
type QueryResult struct {
	Status string `json:"status"`
	Result struct {
		Value json.RawMessage `json:"value"`
		Logs  []string        `json:"logs"`
	} `json:"result"`
	Error string `json:"error,omitempty"`
}

// State returns GET /state.
func (c *Client) State(ctx context.Context) (Card, error) {
	var card Card
	err := c.do(ctx, http.MethodGet, "/state", nil, &card)
	return card, err
}

// StateText returns GET /state?format=text, the card as models read it.
func (c *Client) StateText(ctx context.Context) (string, error) {
	var text string
	err := c.do(ctx, http.MethodGet, "/state?format=text", nil, &text)
	return text, err
}

// Action starts an action (POST /action) and returns its initial status.
// Use Stop for stop actions; their answer is not an ActionStatus.
func (c *Client) Action(ctx context.Context, req any) (*ActionStatus, error) {
	var status ActionStatus
	if err := c.do(ctx, http.MethodPost, "/action", req, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// Eval runs code as the current action (POST /eval), replacing any running
// action, and returns its final status.
func (c *Client) Eval(ctx context.Context, code string, timeoutMs int) (ActionStatus, error) {
	var status ActionStatus
	err := c.do(ctx, http.MethodPost, "/eval", evalRequest(code, timeoutMs), &status)
	return status, err
}

// Query runs read-only code beside the current action (POST /query).
func (c *Client) Query(ctx context.Context, code string, timeoutMs int) (QueryResult, error) {
	var res QueryResult
	err := c.do(ctx, http.MethodPost, "/query", evalRequest(code, timeoutMs), &res)
	return res, err
}

// Events returns body events after seq since.
func (c *Client) Events(ctx context.Context, since int) (EventPage, error) {
	var page EventPage
	err := c.do(ctx, http.MethodGet, "/events?since="+strconv.Itoa(since), nil, &page)
	return page, err
}

// Stop aborts the running action and halts the bot (POST /stop).
func (c *Client) Stop(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/stop", nil, nil)
}

func evalRequest(code string, timeoutMs int) map[string]any {
	req := map[string]any{"code": code}
	if timeoutMs > 0 {
		req["timeoutMs"] = timeoutMs
	}
	return req
}

// do sends one request. out may be *string for text answers, nil to discard.
func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	u, err := url.Parse(strings.TrimRight(c.URL, "/") + path)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("content-type", "application/json")
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) != nil || e.Error == "" {
			e.Error = strings.TrimSpace(string(data))
		}
		return &APIError{Status: resp.StatusCode, Message: e.Error}
	}
	switch out := out.(type) {
	case nil:
		return nil
	case *string:
		*out = string(data)
		return nil
	default:
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("body: %s %s: %w", method, path, err)
		}
		return nil
	}
}
