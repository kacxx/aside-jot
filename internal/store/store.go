// Package store persists jots in SQLite.
//
// Every connection is opened with busy_timeout and WAL set in the DSN, so
// concurrent hook processes queue on the write lock instead of failing
// immediately. Each capture is a single autocommit INSERT.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Statuses an entry can have.
const (
	StatusInbox = "inbox"
	StatusDone  = "done"
)

// ErrNotFound is returned when an entry id does not exist.
var ErrNotFound = errors.New("entry not found")

const timeLayout = "2006-01-02T15:04:05.000Z07:00"

const schemaVersion = 1

const schema = `
CREATE TABLE IF NOT EXISTS entries (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	text       TEXT NOT NULL,
	created_at TEXT NOT NULL,
	status     TEXT NOT NULL DEFAULT 'inbox',
	source     TEXT NOT NULL DEFAULT '',
	session_id TEXT NOT NULL DEFAULT '',
	cwd        TEXT NOT NULL DEFAULT '',
	repo_root  TEXT NOT NULL DEFAULT '',
	repo_name  TEXT NOT NULL DEFAULT '',
	branch     TEXT NOT NULL DEFAULT '',
	commit_sha TEXT NOT NULL DEFAULT '',
	metadata   TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(metadata))
);
CREATE INDEX IF NOT EXISTS entries_status_id ON entries(status, id);
PRAGMA user_version = 1;
`

// Entry is one captured thought.
type Entry struct {
	ID        int64           `json:"id"`
	Text      string          `json:"text"`
	CreatedAt time.Time       `json:"created_at"`
	Status    string          `json:"status"`
	Source    string          `json:"source,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Cwd       string          `json:"cwd,omitempty"`
	RepoRoot  string          `json:"repo_root,omitempty"`
	RepoName  string          `json:"repo_name,omitempty"`
	Branch    string          `json:"branch,omitempty"`
	CommitSHA string          `json:"commit_sha,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

// Store is a handle on the jot database.
type Store struct {
	db *sql.DB
}

// DSN builds the driver data source name for path.
func DSN(path string, busyTimeout time.Duration) string {
	p := filepath.ToSlash(path)
	if runtime.GOOS == "windows" && !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	u := url.URL{Scheme: "file", Path: p}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout("+strconv.FormatInt(busyTimeout.Milliseconds(), 10)+")")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Set("_txlock", "immediate")
	return u.String() + "?" + q.Encode()
}

// Open opens (creating if needed) the database at path.
func Open(path string, busyTimeout time.Duration) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", DSN(path, busyTimeout))
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return s, nil
}

// migrate creates the schema. It only takes the write lock when the schema is
// missing, so opening an existing database is read-only.
func (s *Store) migrate() error {
	var v int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	if v >= schemaVersion {
		return nil
	}
	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the underlying handle for tests and diagnostics.
func (s *Store) DB() *sql.DB { return s.db }

// Insert stores e with one autocommit INSERT and returns its id.
// Status defaults to inbox and CreatedAt to now.
func (s *Store) Insert(ctx context.Context, e *Entry) (int64, error) {
	if e.Status == "" {
		e.Status = StatusInbox
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	meta := string(e.Metadata)
	if meta == "" || meta == "null" {
		meta = "{}"
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO entries
		(text, created_at, status, source, session_id, cwd, repo_root, repo_name, branch, commit_sha, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Text, e.CreatedAt.UTC().Format(timeLayout), e.Status, e.Source, e.SessionID, e.Cwd,
		e.RepoRoot, e.RepoName, e.Branch, e.CommitSHA, meta)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	e.ID = id
	return id, nil
}

const columns = `id, text, created_at, status, source, session_id, cwd, repo_root, repo_name, branch, commit_sha, metadata`

// Get returns the entry with the given id.
func (s *Store) Get(ctx context.Context, id int64) (Entry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+columns+` FROM entries WHERE id = ?`, id)
	if err != nil {
		return Entry{}, err
	}
	es, err := scan(rows)
	if err != nil {
		return Entry{}, err
	}
	if len(es) == 0 {
		return Entry{}, ErrNotFound
	}
	return es[0], nil
}

// List returns the newest entries with the given status (all if empty).
func (s *Store) List(ctx context.Context, status string, limit int) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+columns+` FROM entries
		WHERE (? = '' OR status = ?) ORDER BY id DESC LIMIT ?`, status, status, limitOrAll(limit))
	if err != nil {
		return nil, err
	}
	return scan(rows)
}

// Search returns the newest entries whose text contains q (case-insensitive
// for ASCII). LIKE wildcards in q are matched literally.
func (s *Store) Search(ctx context.Context, q string, limit int) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+columns+` FROM entries
		WHERE text LIKE ? ESCAPE '\' ORDER BY id DESC LIMIT ?`, "%"+EscapeLike(q)+"%", limitOrAll(limit))
	if err != nil {
		return nil, err
	}
	return scan(rows)
}

// SetStatus changes an entry's status.
func (s *Store) SetStatus(ctx context.Context, id int64, status string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE entries SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Backup writes a consistent copy of the database to dst using VACUUM INTO.
// It refuses to overwrite an existing file.
func (s *Store) Backup(ctx context.Context, dst string) error {
	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("backup: %s already exists; refusing to overwrite", dst)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("backup: %w", err)
	}
	_, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, dst)
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	return nil
}

// EscapeLike escapes the LIKE metacharacters %, _ and the escape character \.
func EscapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func limitOrAll(n int) int {
	if n <= 0 {
		return -1
	}
	return n
}

func scan(rows *sql.Rows) ([]Entry, error) {
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		var created, meta string
		if err := rows.Scan(&e.ID, &e.Text, &created, &e.Status, &e.Source, &e.SessionID, &e.Cwd,
			&e.RepoRoot, &e.RepoName, &e.Branch, &e.CommitSHA, &meta); err != nil {
			return nil, err
		}
		t, err := time.Parse(timeLayout, created)
		if err != nil {
			return nil, fmt.Errorf("entry %d: bad created_at %q: %w", e.ID, created, err)
		}
		e.CreatedAt = t
		if meta != "" && meta != "{}" {
			e.Metadata = json.RawMessage(meta)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
