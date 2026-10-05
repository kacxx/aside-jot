package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"sync"
	"time"
)

// Open links take a user back to the chat a jot came from. They are built
// from ids that come from hook payloads and Claude Desktop's own files, so an
// id is checked before it is put in a link, and a link is only built for the
// routes that were tested:
//
//   - Codex app: codex://threads/<session id> (documented).
//   - Claude Desktop, when the session's entrypoint is exactly
//     "claude-desktop": claude://code/continue?session=local_<uuid>
//     (undocumented; see docs/decisions.md). Desktop's id is found in its
//     session file, which also says whether the session is archived.
//
// Anything else has no link, and the caller shows the resume command.
var (
	uuidShape         = regexp.MustCompile(`^(?i:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)
	desktopIDShape    = regexp.MustCompile(`^local_(?i:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)
	cliSessionKey     = regexp.MustCompile(`"cliSessionId"\s*:\s*"([^"\\]*)"`)
	isArchivedKey     = regexp.MustCompile(`"isArchived"\s*:\s*(true|false)`)
	desktopEntrypoint = "claude-desktop"
)

// headerSize is how much of a Desktop session file is read to find its keys.
// Both are within the first ~1.1 KB of every file seen; the files are
// otherwise hundreds of KB of settings aside has no need to read.
const headerSize = 4096

// linkRef is what a jot (or a session's jots) says about where it came from.
type linkRef struct {
	source, sessionID string
	desktopHint       string // claude_desktop_session_id from the hook, unchecked
	entrypoint        string
	transcript        string
}

type desktopSession struct {
	localID  string
	archived bool
}

// desktopLinks finds Claude Desktop's session files. misses lasts as long as
// the Service, so a deleted session doesn't cost a full scan on every call.
type desktopLinks struct {
	mu     sync.Mutex
	dir    string
	misses map[string]bool
}

// SetDesktopSessionsDir sets the directory holding Claude Desktop's session
// files (<dir>/*/*/local_*.json). The default is macOS's; elsewhere it is
// empty and Claude Desktop jots get no link.
func (s *Service) SetDesktopSessionsDir(dir string) {
	s.desktop.mu.Lock()
	defer s.desktop.mu.Unlock()
	s.desktop.dir = dir
}

func defaultDesktopDir() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", "Claude", "claude-code-sessions")
}

// AddOpenURLs sets OpenURL on each entry that has one. It looks Claude
// Desktop's session files up only if an entry came from there.
func (s *Service) AddOpenURLs(es []Entry) {
	refs := make([]linkRef, len(es))
	for i, e := range es {
		refs[i] = refFor([]Entry{e})
	}
	for i, u := range s.openURLs(refs) {
		es[i].OpenURL = u
	}
}

// addSessionOpenURLs sets OpenURL on each session.
func (s *Service) addSessionOpenURLs(ss []Session) {
	refs := make([]linkRef, len(ss))
	for i, x := range ss {
		newestFirst := make([]Entry, len(x.Jots))
		for j, e := range x.Jots {
			newestFirst[len(x.Jots)-1-j] = e
		}
		refs[i] = refFor(newestFirst)
	}
	for i, u := range s.openURLs(refs) {
		ss[i].OpenURL = u
	}
}

// refFor merges what es (newest first) record about one session, taking each
// field from the newest jot that has it.
func refFor(es []Entry) linkRef {
	var r linkRef
	for _, e := range es {
		r.source, r.sessionID = e.Source, e.SessionID
		var m struct {
			Transcript string `json:"transcript_path"`
			Desktop    string `json:"claude_desktop_session_id"`
			Entrypoint string `json:"claude_entrypoint"`
		}
		if json.Unmarshal(e.Metadata, &m) != nil {
			continue
		}
		if r.desktopHint == "" {
			r.desktopHint = m.Desktop
		}
		if r.entrypoint == "" {
			r.entrypoint = m.Entrypoint
		}
		if r.transcript == "" {
			r.transcript = m.Transcript
		}
	}
	return r
}

func (s *Service) openURLs(refs []linkRef) []string {
	out := make([]string, len(refs))
	want := map[string]string{} // Claude session id -> Desktop id hint
	for i, r := range refs {
		switch {
		case !uuidShape.MatchString(r.sessionID):
		case r.source == "codex":
			out[i] = "codex://threads/" + url.PathEscape(r.sessionID)
		case r.source == "claude" && s.desktop.dir != "" && claudeEntrypoint(r) == desktopEntrypoint:
			want[r.sessionID] = r.desktopHint
		}
	}
	found := s.desktop.find(want)
	for i, r := range refs {
		if d, ok := found[r.sessionID]; ok && out[i] == "" && !d.archived {
			out[i] = "claude://code/continue?session=" + url.QueryEscape(d.localID)
		}
	}
	return out
}

// claudeEntrypoint is the entrypoint the hook recorded or, for a jot captured
// before it was recorded, the one in the session's transcript.
func claudeEntrypoint(r linkRef) string {
	if r.entrypoint != "" || r.transcript == "" {
		return r.entrypoint
	}
	return transcriptEntrypoint(r.transcript)
}

// transcriptEntrypoint returns the first entrypoint recorded in a Claude Code
// transcript, or "" if there is none in its first records.
func transcriptEntrypoint(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	r := bufio.NewReader(f)
	for range 100 {
		line, err := r.ReadBytes('\n')
		var rec struct {
			Entrypoint string `json:"entrypoint"`
		}
		if json.Unmarshal(line, &rec) == nil && rec.Entrypoint != "" {
			return rec.Entrypoint
		}
		if err != nil {
			return ""
		}
	}
	return ""
}

// find returns the Desktop session for each Claude session id in want (id to
// the Desktop id the hook recorded, or ""). A hint is checked against its
// file first; the rest are looked for newest file first, stopping once all
// are found. An id with no file is remembered as missing.
func (d *desktopLinks) find(want map[string]string) map[string]desktopSession {
	d.mu.Lock()
	defer d.mu.Unlock()
	found := map[string]desktopSession{}
	if len(want) == 0 || d.dir == "" {
		return found
	}
	files, _ := filepath.Glob(filepath.Join(d.dir, "*", "*", "local_*.json"))
	pending := map[string]bool{}
	for id, hint := range want {
		if d.misses[id] {
			continue
		}
		pending[id] = true
		if !desktopIDShape.MatchString(hint) {
			continue
		}
		for _, f := range files {
			if filepath.Base(f) != hint+".json" {
				continue
			}
			if cli, archived, ok := readHeader(f); ok && cli == id {
				found[id] = desktopSession{hint, archived}
				delete(pending, id)
			}
			break
		}
	}
	if len(pending) > 0 {
		type file struct {
			path string
			mod  time.Time
		}
		byAge := make([]file, 0, len(files))
		for _, f := range files {
			if fi, err := os.Stat(f); err == nil {
				byAge = append(byAge, file{f, fi.ModTime()})
			}
		}
		sort.Slice(byAge, func(i, j int) bool { return byAge[i].mod.After(byAge[j].mod) })
		for _, f := range byAge {
			if len(pending) == 0 {
				break
			}
			cli, archived, ok := readHeader(f.path)
			local := filepath.Base(f.path[:len(f.path)-len(".json")])
			if ok && pending[cli] && desktopIDShape.MatchString(local) {
				found[cli] = desktopSession{local, archived}
				delete(pending, cli)
			}
		}
		if d.misses == nil {
			d.misses = map[string]bool{}
		}
		for id := range pending {
			d.misses[id] = true
		}
	}
	return found
}

// readHeader returns a Desktop session file's cliSessionId and isArchived. It
// reads the first headerSize bytes, and the whole file only if either key is
// not in them.
func readHeader(path string) (cli string, archived, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, false
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, headerSize))
	c, a := cliSessionKey.FindSubmatch(b), isArchivedKey.FindSubmatch(b)
	if c == nil || a == nil {
		rest, _ := io.ReadAll(f)
		b = append(b, rest...)
		c, a = cliSessionKey.FindSubmatch(b), isArchivedKey.FindSubmatch(b)
	}
	if c == nil || a == nil {
		return "", false, false
	}
	return string(c[1]), bytes.Equal(a[1], []byte("true")), true
}
