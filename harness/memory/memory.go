// Package memory owns Clyde's durable episodes, scheduler checkpoint, observed
// places and bounded, episode-backed semantic notes. Runtime files are private
// local data, not part of the trusted prompts or skill library.
package memory

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store owns a SQLite connection and a directory of semantic Markdown files.
// One serve process must own a store; the CLI takes an exclusive process lock.
type Store struct {
	db  *sql.DB
	dir string
	mu  sync.Mutex // semantic file quota and atomic replacement
}

// Episode is a compact observation or result, never an executor transcript.
type Episode struct {
	ID      int64     `json:"id"`
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"`
	Detail  string    `json:"detail"`
	Tokens  int       `json:"tokens,omitempty"`
	CostUSD float64   `json:"cost_usd,omitempty"`
}

// Place is an observation made by code from a body card, not model output.
type Place struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"` // player, home, poi, resource
	Dimension string `json:"dimension"`
	X, Y, Z   float64
	At        time.Time `json:"at"`
}

// Open creates the database schema and semantic directory.
func Open(path, semanticDir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(semanticDir, 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS checkpoint (id INTEGER PRIMARY KEY CHECK(id=1), data BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS episodes (id INTEGER PRIMARY KEY, at TEXT NOT NULL, kind TEXT NOT NULL, detail TEXT NOT NULL, tokens INTEGER NOT NULL, cost REAL NOT NULL);
CREATE TABLE IF NOT EXISTS events (key TEXT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS places (name TEXT PRIMARY KEY, data BLOB NOT NULL);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, dir: semanticDir}, nil
}

// Close releases the database connection.
func (s *Store) Close() error { return s.db.Close() }

// Load restores the last atomic scheduler checkpoint. An empty store leaves dst unchanged.
func (s *Store) Load(ctx context.Context, dst any) error {
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT data FROM checkpoint WHERE id=1`).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

// Commit atomically records the checkpoint, accepted event identities and episodes.
// IDs are assigned to entries in episodes after commit, for semantic citations.
func (s *Store) Commit(ctx context.Context, state any, episodes []Episode, eventKeys ...string) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO checkpoint VALUES(1,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data`, data); err != nil {
		return err
	}
	for _, key := range eventKeys {
		if _, err := tx.ExecContext(ctx, `INSERT INTO events VALUES(?)`, key); err != nil {
			return err
		}
	}
	for i := range episodes {
		e := &episodes[i]
		if e.At.IsZero() {
			e.At = time.Now().UTC()
		}
		r, err := tx.ExecContext(ctx, `INSERT INTO episodes(at,kind,detail,tokens,cost) VALUES(?,?,?,?,?)`, e.At.Format(time.RFC3339Nano), e.Kind, clip(e.Detail, 8192), e.Tokens, e.CostUSD)
		if err != nil {
			return err
		}
		e.ID, err = r.LastInsertId()
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Seen reports whether an event was already accepted, including across body restarts.
func (s *Store) Seen(ctx context.Context, key string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE key=?`, key).Scan(&n)
	return n != 0, err
}

// Recent returns at most 32 compact episodes, newest first.
func (s *Store) Recent(ctx context.Context, limit int) ([]Episode, error) {
	if limit < 1 || limit > 32 {
		limit = 32
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,at,kind,detail,tokens,cost FROM episodes ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Episode
	for rows.Next() {
		var e Episode
		var at string
		if err := rows.Scan(&e.ID, &at, &e.Kind, &e.Detail, &e.Tokens, &e.CostUSD); err != nil {
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

// Observe records a code-observed location. Models are never given this API.
func (s *Store) Observe(ctx context.Context, p Place) error {
	if p.Name == "" || p.Dimension == "" || p.At.IsZero() {
		return errors.New("memory: incomplete observation")
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	query := `INSERT INTO places VALUES(?,?) ON CONFLICT(name) DO UPDATE SET data=excluded.data`
	if p.Kind == "home" {
		query = `INSERT INTO places VALUES(?,?) ON CONFLICT(name) DO NOTHING`
	}
	_, err = s.db.ExecContext(ctx, query, p.Name, b)
	return err
}

// Places returns bounded recent spatial observations.
func (s *Store) Places(ctx context.Context) ([]Place, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM places
ORDER BY (json_extract(data,'$.kind')='home') DESC,
  julianday(json_extract(data,'$.at')) DESC, name LIMIT 64`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Place
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		var p Place
		if err := json.Unmarshal(b, &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

var noteName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)

const (
	MaxNoteBytes     = 4096
	MaxSemanticBytes = 16384
)

// Curate replaces a named note with bounded Markdown and verified episode IDs.
// os.Root confines access even when an existing entry is a symlink.
func (s *Store) Curate(ctx context.Context, name, markdown string, episodes []int64) error {
	if !noteName.MatchString(name) || len(episodes) == 0 || len(episodes) > 16 {
		return errors.New("memory: invalid note name or citations")
	}
	var refs []string
	for _, id := range episodes {
		var n int
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM episodes WHERE id=?`, id).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("memory: unknown episode %d", id)
		}
		refs = append(refs, fmt.Sprint(id))
	}
	data := "Supporting episodes: " + strings.Join(refs, ", ") + "\n\n" + strings.TrimSpace(markdown) + "\n"
	if len(data) > MaxNoteBytes {
		return errors.New("memory: note exceeds 4096 bytes")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return err
	}
	size := len(data)
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".md") && entry.Name() != name+".md" {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			size += int(info.Size())
		}
	}
	if size > MaxSemanticBytes {
		return errors.New("memory: semantic quota exceeded")
	}
	tmp := ".note-" + rand.Text() + ".tmp"
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	_, writeErr := f.WriteString(data)
	err = errors.Join(writeErr, f.Sync(), f.Close())
	if err != nil {
		return err
	}
	return root.Rename(tmp, name+".md")
}

// Semantic reads only bounded Markdown, never arbitrary paths or instructions.
func (s *Store) Semantic() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		f, err := root.Open(entry.Name())
		if err != nil {
			return "", err
		}
		b, readErr := io.ReadAll(io.LimitReader(f, MaxNoteBytes+1))
		err = errors.Join(readErr, f.Close())
		if err != nil {
			return "", err
		}
		if len(b) > MaxNoteBytes || out.Len()+len(b) > MaxSemanticBytes {
			return "", errors.New("memory: semantic files exceed quota")
		}
		out.Write(b)
	}
	return out.String(), nil
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
