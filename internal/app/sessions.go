package app

import (
	"context"
	"errors"
	"sort"
	"strings"
)

// SessionPrefix starts a jot that labels its session (">> session: SUP-4821 …").
const SessionPrefix = "session:"

// Session is the jots captured in one agent conversation.
type Session struct {
	Source string
	ID     string
	// Cwd is the working directory of the session's first jot that has one.
	Cwd   string
	Jots  []Entry // oldest first
	Label Entry   // the newest "session:" jot, else the first jot
	// Matches are the session's jots that matched a Find query, oldest first.
	Matches []Entry
}

// Latest returns the session's newest jot.
func (s Session) Latest() Entry { return s.Jots[len(s.Jots)-1] }

// LabelText is the label's first line, without a "session:" prefix.
func (s Session) LabelText() string {
	line, _, _ := strings.Cut(s.Label.Text, "\n")
	if rest, ok := cutSessionPrefix(line); ok {
		return rest
	}
	return line
}

// ResumeCommand returns a shell command that reopens the session, or "" if
// its agent has no resume command aside knows about.
func (s Session) ResumeCommand() string {
	var cmd string
	switch s.Source {
	case "claude":
		cmd = "claude --resume " + shellQuote(s.ID)
	case "codex":
		cmd = "codex resume " + shellQuote(s.ID)
	default:
		return ""
	}
	if s.Cwd == "" {
		return cmd
	}
	// Claude Code stores sessions per project directory, so resume from there.
	return "cd " + shellQuote(s.Cwd) + " && " + cmd
}

// Sessions returns the n most recently active sessions (all if n <= 0).
// Jots without a session id, such as those from aside add, are not in any.
func (s *Service) Sessions(ctx context.Context, n int) ([]Session, error) {
	all, err := s.store.List(ctx, "", 0)
	if err != nil {
		return nil, err
	}
	ss := groupSessions(all)
	if n > 0 && len(ss) > n {
		ss = ss[:n]
	}
	return ss, nil
}

// Find returns the sessions with a jot containing q, most recently active
// first, each with its matching jots, plus the matching jots that have no
// session. Matching is the same as Search.
func (s *Service) Find(ctx context.Context, q string) ([]Session, []Entry, error) {
	if strings.TrimSpace(q) == "" {
		return nil, nil, errors.New("empty search query")
	}
	matches, err := s.store.Search(ctx, q, 0)
	if err != nil {
		return nil, nil, err
	}
	all, err := s.store.List(ctx, "", 0)
	if err != nil {
		return nil, nil, err
	}
	var loose []Entry
	for _, e := range matches { // newest first, as Search returns them
		if e.SessionID == "" {
			loose = append(loose, e)
		}
	}
	byKey := map[sessionKey][]Entry{}
	for i := len(matches) - 1; i >= 0; i-- { // oldest first within a session
		if e := matches[i]; e.SessionID != "" {
			byKey[keyOf(e)] = append(byKey[keyOf(e)], e)
		}
	}
	var found []Session
	for _, ss := range groupSessions(all) {
		if m, ok := byKey[sessionKey{ss.Source, ss.ID}]; ok {
			ss.Matches = m
			found = append(found, ss)
		}
	}
	return found, loose, nil
}

type sessionKey struct{ source, id string }

func keyOf(e Entry) sessionKey { return sessionKey{e.Source, e.SessionID} }

// groupSessions groups entries (any order) by source and session id, most
// recently active session first.
func groupSessions(es []Entry) []Session {
	sorted := append([]Entry(nil), es...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	idx := map[sessionKey]int{}
	var ss []Session
	for _, e := range sorted {
		if e.SessionID == "" {
			continue
		}
		k := keyOf(e)
		i, ok := idx[k]
		if !ok {
			i = len(ss)
			idx[k] = i
			ss = append(ss, Session{Source: e.Source, ID: e.SessionID})
		}
		ss[i].Jots = append(ss[i].Jots, e)
		if ss[i].Cwd == "" {
			ss[i].Cwd = e.Cwd
		}
	}
	for i := range ss {
		ss[i].Label = ss[i].Jots[0]
		for _, e := range ss[i].Jots {
			if _, ok := cutSessionPrefix(e.Text); ok {
				ss[i].Label = e
			}
		}
	}
	sort.SliceStable(ss, func(i, j int) bool { return ss[i].Latest().ID > ss[j].Latest().ID })
	return ss
}

// cutSessionPrefix reports whether text starts with "session:" (any case) and
// returns the rest, trimmed.
func cutSessionPrefix(text string) (string, bool) {
	if len(text) < len(SessionPrefix) || !strings.EqualFold(text[:len(SessionPrefix)], SessionPrefix) {
		return "", false
	}
	return strings.TrimSpace(text[len(SessionPrefix):]), true
}

// shellQuote quotes s for a POSIX shell unless it only has safe characters.
func shellQuote(s string) string {
	safe := func(r rune) bool {
		return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./:@%+=,", r)
	}
	if s != "" && strings.IndexFunc(s, func(r rune) bool { return !safe(r) }) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
