// Package store persists jots in SQLite.
//
// Every connection is opened with busy_timeout, WAL and synchronous=NORMAL set
// in the DSN, so concurrent hook processes queue on the write lock instead of
// failing immediately, and each holds it only briefly. Each capture is a
// single autocommit INSERT.
//
// busy_timeout is not the whole story for a brand-new file: switching it to
// WAL can return SQLITE_BUSY without consulting the busy handler when several
// processes create the database at once. Open therefore retries, bounding every
// attempt by what is left of the busy timeout. Open any database through Open,
// or that race comes back.
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

	"modernc.org/sqlite" // also registers the "sqlite" driver
)

// sqliteBusy is SQLITE_BUSY, the primary result code for a locked database.
const sqliteBusy = 5

// openRetryInterval is the pause between Open attempts that failed with
// SQLITE_BUSY. Short, because those failures are quick and the busy-timeout
// budget is usually a couple of seconds; attempts are bounded by what is left
// of that budget either way.
const openRetryInterval = 10 * time.Millisecond

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
	// IssueURL is metadata's issue_url, set when the jot was promoted to an
	// issue. It is read from metadata, never written by Insert.
	IssueURL string `json:"issue_url,omitempty"`
}

// MetaIssueURL is the metadata key holding a promoted jot's issue URL.
const MetaIssueURL = "issue_url"

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
	// In WAL mode NORMAL syncs at checkpoints rather than on every commit, so
	// a writer no longer holds the write lock across a disk flush. With FULL,
	// slow flushes (Windows) let back-to-back writers starve a waiting
	// connection past its busy timeout. A commit survives an application
	// crash either way; only a power loss or OS crash can undo the newest ones.
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Set("_txlock", "immediate")
	return u.String() + "?" + q.Encode()
}

// Open opens (creating if needed) the database at path. It waits at most
// busyTimeout for a locked database, including while creating it.
func Open(path string, busyTimeout time.Duration) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
	}
	if err := secureDBFiles(path); err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	// Each attempt gets its own handle whose busy_timeout is what is left of
	// the budget, so neither the connect-time WAL switch nor BEGIN IMMEDIATE
	// can wait past the deadline.
	deadline := time.Now().Add(busyTimeout)
	for {
		err := openAttempt(path, max(time.Until(deadline), 0))
		if err == nil {
			break
		}
		remaining := time.Until(deadline)
		if !isBusy(err) || remaining <= 0 {
			return nil, fmt.Errorf("open %s: %w", path, err)
		}
		time.Sleep(min(openRetryInterval, remaining))
	}
	// The file is now WAL with the current schema, so the pragmas in the DSN
	// no longer need a lock.
	db, err := sql.Open("sqlite", DSN(path, busyTimeout))
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

// openAttempt makes one attempt at preparing the database; a variable so tests
// can simulate the WAL-switch race.
var openAttempt = migrateWithTimeout

// migrateWithTimeout connects with the given busy timeout and ensures the
// schema exists.
func migrateWithTimeout(path string, busyTimeout time.Duration) error {
	db, err := sql.Open("sqlite", DSN(path, busyTimeout))
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	return migrate(db)
}

// migrate creates the schema. It only takes the write lock when the schema is
// missing, so opening an existing database is read-only.
func migrate(db *sql.DB) error {
	if v, err := userVersion(db); err != nil || v >= schemaVersion {
		return err
	}
	// BEGIN IMMEDIATE (see _txlock in DSN) takes the write lock up front, where
	// busy_timeout applies, and serialises concurrent first opens. Re-check the
	// version under the lock in case another process migrated meanwhile.
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // after Commit this is a no-op returning ErrTxDone
	if v, err := userVersion(tx); err != nil || v >= schemaVersion {
		return err
	}
	if _, err := tx.Exec(schema); err != nil {
		return err
	}
	return tx.Commit()
}

// userVersion reads PRAGMA user_version from a *sql.DB or *sql.Tx.
func userVersion(q interface {
	QueryRow(query string, args ...any) *sql.Row
}) (int, error) {
	var v int
	err := q.QueryRow("PRAGMA user_version").Scan(&v)
	return v, err
}

func isBusy(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code()&0xff == sqliteBusy
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

// SetMetadata sets metadata[key] = value and, unless status is empty, the
// status, in one transaction. Other metadata keys are kept.
func (s *Store) SetMetadata(ctx context.Context, id int64, key string, value any, status string) error {
	v, err := json.Marshal(value)
	if err != nil {
		return err
	}
	// BEGIN IMMEDIATE (see _txlock in DSN): the read below and the update can't
	// interleave with another writer.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // after Commit this is a no-op returning ErrTxDone
	var raw, cur string
	err = tx.QueryRowContext(ctx, `SELECT metadata, status FROM entries WHERE id = ?`, id).Scan(&raw, &cur)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	meta := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(raw), &meta); err != nil || meta == nil {
		return fmt.Errorf("entry %d: metadata is not a JSON object", id)
	}
	meta[key] = v
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if status == "" {
		status = cur
	}
	if _, err := tx.ExecContext(ctx, `UPDATE entries SET metadata = ?, status = ? WHERE id = ?`,
		string(b), status, id); err != nil {
		return err
	}
	return tx.Commit()
}

// Backup writes a consistent copy of the database to dst using VACUUM INTO.
// It refuses to overwrite an existing file. The backup is owner-only: dst is
// created exclusively with filePerm first, and VACUUM INTO accepts an empty
// file, so there is no moment when the copy is readable by others.
func (s *Store) Backup(ctx context.Context, dst string) (err error) {
	if strings.TrimSpace(dst) == "" {
		return errors.New("backup: destination path is empty")
	}
	// VACUUM INTO reads a destination starting with "file:" as a URI, which
	// would write the copy somewhere other than the file created below. An
	// absolute path never starts with "file:".
	if dst, err = filepath.Abs(dst); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("backup: %s already exists; refusing to overwrite", dst)
	}
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	// From here on dst is ours: remove it on any failure.
	defer func() {
		if err != nil {
			_ = os.Remove(dst)
		}
	}()
	if err := f.Close(); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, dst); err != nil {
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
			var m map[string]any
			if json.Unmarshal(e.Metadata, &m) == nil {
				e.IssueURL, _ = m[MetaIssueURL].(string)
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
