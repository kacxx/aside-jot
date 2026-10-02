// Package app is the single entry point the CLI, hooks and MCP server use.
// It owns the store and git lookups; callers never touch them directly.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/kacxx/aside-jot/internal/gitctx"
	"github.com/kacxx/aside-jot/internal/store"
)

// Entry is a captured thought.
type Entry = store.Entry

// ErrNotFound is returned for an unknown entry id.
var ErrNotFound = store.ErrNotFound

// Opener lazily opens a Service. Hooks call it only once a prompt is known to
// be a jot, so ordinary prompts never touch SQLite or git.
type Opener func() (*Service, error)

// Service implements jot's use cases.
type Service struct {
	store *store.Store
	git   func(ctx context.Context, dir string) gitctx.Info
	now   func() time.Time
}

// New wraps an open store.
func New(st *store.Store) *Service {
	return &Service{store: st, git: gitctx.Detect, now: time.Now}
}

// Open opens the database at path.
func Open(path string, busyTimeout time.Duration) (*Service, error) {
	st, err := store.Open(path, busyTimeout)
	if err != nil {
		return nil, err
	}
	return New(st), nil
}

// OpenDefault opens the database chosen by the environment (see DBPath).
func OpenDefault() (*Service, error) {
	path, err := DBPath()
	if err != nil {
		return nil, err
	}
	return Open(path, BusyTimeout())
}

// Close releases the database.
func (s *Service) Close() error { return s.store.Close() }

// CaptureRequest describes one jot.
type CaptureRequest struct {
	Text      string
	Source    string // "claude", "cursor", "cli"
	SessionID string
	Cwd       string
	Metadata  map[string]any
	// OnceKey, if set, names a string Metadata key that identifies the prompt
	// (e.g. Cursor's generation_id). A second capture from the same Source
	// with the same value returns the first entry instead of saving again.
	OnceKey string
}

// Capture stores a jot with best-effort git context for req.Cwd.
func (s *Service) Capture(ctx context.Context, req CaptureRequest) (Entry, error) {
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return Entry{}, errors.New("empty jot")
	}
	g := s.git(ctx, req.Cwd)
	e := Entry{
		Text:      text,
		CreatedAt: s.now(),
		Source:    req.Source,
		SessionID: req.SessionID,
		Cwd:       req.Cwd,
		RepoRoot:  g.Root,
		RepoName:  g.Name,
		Branch:    g.Branch,
		CommitSHA: g.Commit,
	}
	if len(req.Metadata) > 0 {
		b, err := json.Marshal(req.Metadata)
		if err != nil {
			return Entry{}, err
		}
		e.Metadata = b
	}
	if v, _ := req.Metadata[req.OnceKey].(string); req.OnceKey != "" && v != "" {
		if _, err := s.store.InsertOnce(ctx, &e, req.OnceKey, v); err != nil {
			return Entry{}, err
		}
		return e, nil
	}
	if _, err := s.store.Insert(ctx, &e); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// Inbox returns the newest n inbox entries (all if n <= 0).
func (s *Service) Inbox(ctx context.Context, n int) ([]Entry, error) {
	return s.store.List(ctx, store.StatusInbox, n)
}

// Show returns one entry.
func (s *Service) Show(ctx context.Context, id int64) (Entry, error) {
	return s.store.Get(ctx, id)
}

// Search returns the newest n entries containing q, any status.
func (s *Service) Search(ctx context.Context, q string, n int) ([]Entry, error) {
	if strings.TrimSpace(q) == "" {
		return nil, errors.New("empty search query")
	}
	return s.store.Search(ctx, q, n)
}

// Done marks an entry as done.
func (s *Service) Done(ctx context.Context, id int64) error {
	return s.store.SetStatus(ctx, id, store.StatusDone)
}

// Backup writes a consistent copy of the database to dst (never overwrites).
func (s *Service) Backup(ctx context.Context, dst string) error {
	return s.store.Backup(ctx, dst)
}
