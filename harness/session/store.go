// Package session owns the persistent player's journal and runtime. Native
// provider messages are opaque data here, never flattened or reconstructed.
package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	agent "github.com/lox/agent-harness"
	_ "modernc.org/sqlite"
)

const journalVersion = 1

var (
	ErrConflict  = errors.New("session: checkpoint changed")
	ErrStopped   = errors.New("session: operator stop is latched")
	ErrAllowance = errors.New("session: inference allowance exhausted")
)

// Wait suspends inference, not inbox ingestion. A zero Until waits for events.
type Wait struct {
	Until  time.Time `json:"until,omitempty"`
	Reason string    `json:"reason"`
}

// PendingTool distinguishes a possibly dispatched effect from a withheld call.
// Neither is replayed automatically after a restart.
type PendingTool struct {
	Call    agent.ToolCall `json:"call"`
	Started bool           `json:"started"`
}

// State is the bounded working checkpoint, separate from append-only history.
// Resource controls live in SQL columns that checkpoint writes cannot overwrite.
type State struct {
	Messages    []agent.Message `json:"messages,omitempty"`
	Pending     []PendingTool   `json:"pending,omitempty"`
	ActionToken string          `json:"action_token,omitempty"`
	Wait        *Wait           `json:"wait,omitempty"`
	Cursor      int             `json:"cursor"`
	CursorTS    int64           `json:"cursor_ts"`
}

// Snapshot includes authoritative controls and a checkpoint revision for CAS.
type Snapshot struct {
	State       State
	Revision    int64
	Limit, Used int64
	Stopped     bool
}

// Entry is an immutable timestamped history record, addressable by ID.
type Entry struct {
	ID   int64           `json:"id"`
	At   time.Time       `json:"at"`
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

// Input is one deduplicated accepted event. Keys identify source incarnations,
// not just sequence numbers which the body can reuse after restart.
type Input struct {
	ID   int64     `json:"id"`
	Key  string    `json:"key"`
	At   time.Time `json:"at"`
	Kind string    `json:"kind"`
	Text string    `json:"text"`
}

// Receipt records a completed provider request. Nil usage means unknown billing,
// including requests cancelled or failed without returned usage.
type Receipt struct {
	ID    int64
	Usage *agent.Usage
}

// Update publishes state, original history, event consumption and a request
// receipt in one transaction. An error publishes none of them.
type Update struct {
	State   State
	Entries []Entry
	Applied []int64
	Receipt *Receipt
}

// Accounting separates returned token counts from calls with unknown billing.
// It is not a dollar cap: the enforceable allowance counts dispatch reservations.
type Accounting struct {
	Usage   agent.Usage
	Unknown int64
}

// Journal uses independent tables in the existing memory database. Legacy
// checkpoints, episodes, places and semantic files are left intact.
type Journal struct{ db *sql.DB }

// Open opens or creates a journal. Initialize must precede runtime writes.
// Callers still need the serve ownership lease; SQL CAS protects stale writes,
// not two independent body owners.
func Open(path string) (*Journal, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS player_state (
 id INTEGER PRIMARY KEY CHECK(id=1), version INTEGER NOT NULL, revision INTEGER NOT NULL,
 state BLOB NOT NULL, request_limit INTEGER NOT NULL, used INTEGER NOT NULL,
 stopped INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS player_history (
 id INTEGER PRIMARY KEY, at TEXT NOT NULL, kind TEXT NOT NULL, data BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS player_inbox (
 id INTEGER PRIMARY KEY, key TEXT NOT NULL UNIQUE, at TEXT NOT NULL,
 kind TEXT NOT NULL, text TEXT NOT NULL, applied INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS player_requests (
 id INTEGER PRIMARY KEY, at TEXT NOT NULL, purpose TEXT NOT NULL,
 observation TEXT NOT NULL, completed INTEGER NOT NULL DEFAULT 0, usage BLOB);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	j := &Journal{db: db}
	_, err = j.Load(context.Background())
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		db.Close()
		return nil, err
	}
	return j, nil
}

// Close releases the connection. SQLite retains committed reservations on crash.
func (j *Journal) Close() error { return j.db.Close() }

// Initialize grants an allowance only on first creation. Startup configuration
// changes never extend an existing session; only Control can do that.
func (j *Journal) Initialize(ctx context.Context, allowance int64) (Snapshot, error) {
	if allowance < 1 {
		return Snapshot{}, errors.New("session: positive initial allowance required")
	}
	_, err := j.db.ExecContext(ctx, `INSERT INTO player_state VALUES(1,?,0,'{}',?,0,0) ON CONFLICT(id) DO NOTHING`, journalVersion, allowance)
	if err != nil {
		return Snapshot{}, err
	}
	return j.Load(ctx)
}

type rowReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func load(ctx context.Context, db rowReader) (Snapshot, error) {
	var s Snapshot
	var version int
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT version,revision,state,request_limit,used,stopped FROM player_state WHERE id=1`).Scan(&version, &s.Revision, &raw, &s.Limit, &s.Used, &s.Stopped)
	if err != nil {
		return s, err
	}
	if version != journalVersion {
		return s, fmt.Errorf("session: unsupported journal version %d (want %d); use a matching binary or stopped whole-memory backup", version, journalVersion)
	}
	if err := json.Unmarshal(raw, &s.State); err != nil {
		return s, fmt.Errorf("session: decode checkpoint: %w", err)
	}
	return s, nil
}

// Load reads the checkpoint and current operator controls together.
func (j *Journal) Load(ctx context.Context) (Snapshot, error) { return load(ctx, j.db) }

// Commit atomically publishes a runtime transition without altering controls.
func (j *Journal) Commit(ctx context.Context, revision int64, update Update) (Snapshot, error) {
	raw, err := json.Marshal(update.State)
	if err != nil {
		return Snapshot{}, err
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return Snapshot{}, err
	}
	defer tx.Rollback()
	r, err := tx.ExecContext(ctx, `UPDATE player_state SET state=?,revision=revision+1 WHERE id=1 AND version=? AND revision=?`, raw, journalVersion, revision)
	if err != nil {
		return Snapshot{}, err
	}
	if err := changed(r, ErrConflict); err != nil {
		return Snapshot{}, err
	}
	for _, e := range update.Entries {
		if e.Kind == "" || !json.Valid(e.Data) {
			return Snapshot{}, errors.New("session: history requires kind and valid JSON")
		}
		if e.At.IsZero() {
			e.At = time.Now().UTC()
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO player_history(at,kind,data) VALUES(?,?,?)`, e.At.Format(time.RFC3339Nano), e.Kind, []byte(e.Data)); err != nil {
			return Snapshot{}, err
		}
	}
	for _, id := range update.Applied {
		r, err := tx.ExecContext(ctx, `UPDATE player_inbox SET applied=1 WHERE id=? AND applied=0`, id)
		if err != nil {
			return Snapshot{}, err
		}
		if err := changed(r, errors.New("session: inbox event missing or already applied")); err != nil {
			return Snapshot{}, err
		}
	}
	if receipt := update.Receipt; receipt != nil {
		var usage []byte
		if receipt.Usage != nil {
			usage, err = json.Marshal(receipt.Usage)
			if err != nil {
				return Snapshot{}, err
			}
		}
		r, err := tx.ExecContext(ctx, `UPDATE player_requests SET completed=1,usage=? WHERE id=? AND completed=0`, usage, receipt.ID)
		if err != nil {
			return Snapshot{}, err
		}
		if err := changed(r, errors.New("session: reservation missing or already completed")); err != nil {
			return Snapshot{}, err
		}
	}
	s, err := load(ctx, tx)
	if err != nil {
		return Snapshot{}, err
	}
	return s, tx.Commit()
}

func changed(r sql.Result, otherwise error) error {
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return otherwise
	}
	return nil
}

// Accept persists events idempotently. Source polling must not advance its
// durable cursor before acceptance; re-fetching after a crash is harmless.
func (j *Journal) Accept(ctx context.Context, inputs []Input) error {
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, in := range inputs {
		if in.Key == "" || in.Kind == "" || in.At.IsZero() {
			return errors.New("session: incomplete inbox event")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO player_inbox(key,at,kind,text) VALUES(?,?,?,?) ON CONFLICT(key) DO NOTHING`, in.Key, in.At.Format(time.RFC3339Nano), in.Kind, in.Text); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Pending returns a bounded page of events not yet applied to the conversation.
func (j *Journal) Pending(ctx context.Context, after int64, limit int) ([]Input, error) {
	rows, err := j.db.QueryContext(ctx, `SELECT id,key,at,kind,text FROM player_inbox WHERE applied=0 AND id>? ORDER BY id LIMIT ?`, after, min(max(limit, 1), 64))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Input
	for rows.Next() {
		var in Input
		var at string
		if err := rows.Scan(&in.ID, &in.Key, &at, &in.Kind, &in.Text); err != nil {
			return nil, err
		}
		in.At, err = time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

// Reserve charges one inference/compaction request before dispatch. A crash,
// error or cancellation never refunds it. No provider SDK retries may hide
// additional requests behind one reservation.
func (j *Journal) Reserve(ctx context.Context, purpose, observation string) (int64, error) {
	if purpose != "inference" && purpose != "compaction" {
		return 0, errors.New("session: invalid request purpose")
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	r, err := tx.ExecContext(ctx, `UPDATE player_state SET used=used+1 WHERE id=1 AND version=? AND stopped=0 AND used<request_limit`, journalVersion)
	if err != nil {
		return 0, err
	}
	if err := changed(r, ErrAllowance); err != nil {
		s, loadErr := load(ctx, tx)
		if loadErr != nil {
			return 0, loadErr
		}
		if s.Stopped {
			return 0, ErrStopped
		}
		return 0, err
	}
	r, err = tx.ExecContext(ctx, `INSERT INTO player_requests(at,purpose,observation) VALUES(?,?,?)`, time.Now().UTC().Format(time.RFC3339Nano), purpose, observation)
	if err != nil {
		return 0, err
	}
	id, err := r.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// Control is an operator-only operation, never exposed as a model tool. A
// resume does not refill requests. Extension and latch changes are audited.
func (j *Journal) Control(ctx context.Context, stopped bool, extra int64) error {
	if extra < 0 {
		return errors.New("session: allowance extension cannot be negative")
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, err := tx.ExecContext(ctx, `UPDATE player_state SET stopped=?,request_limit=request_limit+? WHERE id=1 AND version=? AND request_limit<=?`, stopped, extra, journalVersion, math.MaxInt64-extra)
	if err != nil {
		return err
	}
	if err := changed(r, errors.New("session: missing/incompatible state or allowance overflow")); err != nil {
		return err
	}
	raw, err := json.Marshal(struct {
		Stopped bool  `json:"stopped"`
		Extra   int64 `json:"extra_requests"`
	}{stopped, extra})
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO player_history(at,kind,data) VALUES(?,?,?)`, time.Now().UTC().Format(time.RFC3339Nano), "operator_control", raw); err != nil {
		return err
	}
	return tx.Commit()
}

// Accounting sums returned usage; every unreceipted or usage-less request stays
// explicitly unknown, including after restart.
func (j *Journal) Accounting(ctx context.Context) (Accounting, error) {
	rows, err := j.db.QueryContext(ctx, `SELECT usage FROM player_requests`)
	if err != nil {
		return Accounting{}, err
	}
	defer rows.Close()
	var out Accounting
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return out, err
		}
		if len(raw) == 0 {
			out.Unknown++
			continue
		}
		var usage agent.Usage
		if err := json.Unmarshal(raw, &usage); err != nil {
			return out, err
		}
		out.Usage.Add(&usage)
	}
	return out, rows.Err()
}

// History reads/searches immutable originals with exclusive-ID pagination.
// The caller must bound rendered tool output without mutating stored originals.
func (j *Journal) History(ctx context.Context, query string, after int64, limit int) ([]Entry, error) {
	rows, err := j.db.QueryContext(ctx, `SELECT id,at,kind,data FROM player_history WHERE id>? AND instr(lower(CAST(data AS TEXT)),lower(?))>0 ORDER BY id LIMIT ?`, after, query, min(max(limit, 1), 64))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		var at string
		if err := rows.Scan(&e.ID, &at, &e.Kind, &e.Data); err != nil {
			return nil, err
		}
		e.At, err = time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
