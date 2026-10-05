package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	cliA   = "11111111-1111-4111-8111-111111111111"
	cliB   = "22222222-2222-4222-8222-222222222222"
	localA = "local_aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	localB = "local_bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

// desktopFile writes a Desktop session file the way Desktop does: the keys
// near the top, then a lot of other data.
func desktopFile(t *testing.T, dir, local, cli string, archived bool, age time.Duration) {
	t.Helper()
	d := filepath.Join(dir, "acct", "org")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(d, local+".json")
	body := fmt.Sprintf(`{"sessionId":%q,"cliSessionId":%q,"isArchived":%v,"mcp":"%s"}`, local, cli, archived, strings.Repeat("x", 8000))
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	mt := time.Now().Add(-age)
	if err := os.Chtimes(p, mt, mt); err != nil {
		t.Fatal(err)
	}
}

func jotMeta(t *testing.T, svc *Service, source, session string, meta map[string]any) Entry {
	t.Helper()
	e, err := svc.Capture(context.Background(), CaptureRequest{Text: "note", Source: source, SessionID: session, Metadata: meta})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func openURL(svc *Service, e Entry) string {
	es := []Entry{e}
	svc.AddOpenURLs(es)
	return es[0].OpenURL
}

func TestOpenURLCodex(t *testing.T) {
	svc := sessionService(t)
	if got := openURL(svc, jotMeta(t, svc, "codex", cliA, nil)); got != "codex://threads/"+cliA {
		t.Fatalf("got %q", got)
	}
	for _, bad := range []string{"s1", "../x", cliA + "/../x", "a b"} {
		if got := openURL(svc, jotMeta(t, svc, "codex", bad, nil)); got != "" {
			t.Fatalf("session %q got %q, want no link", bad, got)
		}
	}
}

func TestOpenURLDesktop(t *testing.T) {
	dir := t.TempDir()
	svc := sessionService(t)
	svc.SetDesktopSessionsDir(dir)
	desktopFile(t, dir, localA, cliA, false, time.Second)
	desktopFile(t, dir, localB, cliB, true, 2*time.Second)
	want := "claude://code/continue?session=" + localA

	// Id recorded at capture.
	hinted := map[string]any{"claude_entrypoint": "claude-desktop", "claude_desktop_session_id": localA}
	if got := openURL(svc, jotMeta(t, svc, "claude", cliA, hinted)); got != want {
		t.Fatalf("hinted: got %q", got)
	}
	// An older jot: no id recorded, found in the session files.
	if got := openURL(svc, jotMeta(t, svc, "claude", cliA, map[string]any{"claude_entrypoint": "claude-desktop"})); got != want {
		t.Fatalf("lookup: got %q", got)
	}
	// A hint that doesn't belong to the session is not trusted.
	wrong := map[string]any{"claude_entrypoint": "claude-desktop", "claude_desktop_session_id": localB}
	if got := openURL(svc, jotMeta(t, svc, "claude", cliA, wrong)); got != want {
		t.Fatalf("wrong hint: got %q", got)
	}
	// Archived sessions have no link.
	if got := openURL(svc, jotMeta(t, svc, "claude", cliB, hinted)); got != "" {
		t.Fatalf("archived: got %q", got)
	}
	// Other entrypoints, including claude-desktop-3p, have none.
	for _, ep := range []string{"cli", "claude-desktop-3p", "sdk-cli", ""} {
		m := map[string]any{"claude_desktop_session_id": localA}
		if ep != "" {
			m["claude_entrypoint"] = ep
		}
		if got := openURL(svc, jotMeta(t, svc, "claude", cliA, m)); got != "" {
			t.Fatalf("entrypoint %q: got %q", ep, got)
		}
	}
	// A session with no file is a miss, remembered once its jot isn't recent.
	svc.desktop.now = func() time.Time { return time.Now().Add(time.Hour) }
	m := map[string]any{"claude_entrypoint": "claude-desktop"}
	if got := openURL(svc, jotMeta(t, svc, "claude", "33333333-3333-4333-8333-333333333333", m)); got != "" {
		t.Fatalf("missing: got %q", got)
	}
	if _, ok := svc.desktop.misses["33333333-3333-4333-8333-333333333333"]; !ok {
		t.Fatal("miss not remembered")
	}
	// Bad ids never reach a link.
	if got := openURL(svc, jotMeta(t, svc, "claude", "s1&x=1", m)); got != "" {
		t.Fatalf("bad id: got %q", got)
	}
	// No sessions directory (not macOS): no link, no error.
	svc.SetDesktopSessionsDir("")
	if got := openURL(svc, jotMeta(t, svc, "claude", cliA, hinted)); got != "" {
		t.Fatalf("no dir: got %q", got)
	}
}

func TestOpenURLEntrypointFromTranscript(t *testing.T) {
	dir := t.TempDir()
	svc := sessionService(t)
	svc.SetDesktopSessionsDir(dir)
	desktopFile(t, dir, localA, cliA, false, time.Second)
	tr := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(tr, []byte("{\"type\":\"summary\"}\n{\"cwd\":\"/x\",\"entrypoint\":\"claude-desktop\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := openURL(svc, jotMeta(t, svc, "claude", cliA, map[string]any{"transcript_path": tr}))
	if got != "claude://code/continue?session="+localA {
		t.Fatalf("got %q", got)
	}
}

func TestOpenURLHeaderBeyondFirstBlock(t *testing.T) {
	dir := t.TempDir()
	svc := sessionService(t)
	svc.SetDesktopSessionsDir(dir)
	d := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"pad":"` + strings.Repeat("x", 6000) + `","cliSessionId":"` + cliA + `","isArchived":false}`
	if err := os.WriteFile(filepath.Join(d, localA+".json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	m := map[string]any{"claude_entrypoint": "claude-desktop"}
	if got := openURL(svc, jotMeta(t, svc, "claude", cliA, m)); got != "claude://code/continue?session="+localA {
		t.Fatalf("got %q", got)
	}
}

func TestFindSetsOpenURL(t *testing.T) {
	ctx := context.Background()
	svc := sessionService(t)
	jotMeta(t, svc, "codex", cliA, nil)
	ss, _, err := svc.Find(ctx, "note")
	if err != nil || len(ss) != 1 || ss[0].OpenURL != "codex://threads/"+cliA {
		t.Fatalf("%+v %v", ss, err)
	}
}

// A jot captured long ago is looked for only in files modified since it was
// captured, and a miss is remembered; a stored id is still checked first.
func TestOpenURLMisses(t *testing.T) {
	dir := t.TempDir()
	svc := sessionService(t)
	svc.SetDesktopSessionsDir(dir)
	clock := time.Now()
	svc.desktop.now = func() time.Time { return clock }
	m := map[string]any{"claude_entrypoint": "claude-desktop"}

	// Not in the files, and not recent: a miss, remembered.
	clock = time.Now().Add(time.Hour)
	if got := openURL(svc, jotMeta(t, svc, "claude", cliA, m)); got != "" {
		t.Fatalf("got %q", got)
	}
	if _, ok := svc.desktop.misses[cliA]; !ok {
		t.Fatal("miss not remembered")
	}
	// The file appears; the remembered miss still holds, until it expires.
	desktopFile(t, dir, localA, cliA, false, 0)
	if got := openURL(svc, jotMeta(t, svc, "claude", cliA, m)); got != "" {
		t.Fatalf("remembered miss: got %q", got)
	}
	// ...but a stored id is checked before the miss cache.
	hinted := map[string]any{"claude_entrypoint": "claude-desktop", "claude_desktop_session_id": localA}
	if got := openURL(svc, jotMeta(t, svc, "claude", cliA, hinted)); got != "claude://code/continue?session="+localA {
		t.Fatalf("hint behind a miss: got %q", got)
	}
	clock = clock.Add(missTTL + time.Minute)
	if got := openURL(svc, jotMeta(t, svc, "claude", cliA, m)); got == "" {
		t.Fatal("expired miss not retried")
	}
}

// A session whose file isn't written yet, for a jot just captured, is not
// remembered as missing.
func TestOpenURLFreshMissNotRemembered(t *testing.T) {
	dir := t.TempDir()
	svc := sessionService(t)
	svc.SetDesktopSessionsDir(dir)
	m := map[string]any{"claude_entrypoint": "claude-desktop"}
	if got := openURL(svc, jotMeta(t, svc, "claude", cliA, m)); got != "" {
		t.Fatalf("got %q", got)
	}
	if _, ok := svc.desktop.misses[cliA]; ok {
		t.Fatal("a fresh jot's miss was remembered")
	}
	desktopFile(t, dir, localA, cliA, false, 0)
	if got := openURL(svc, jotMeta(t, svc, "claude", cliA, m)); got != "claude://code/continue?session="+localA {
		t.Fatalf("after the file appeared: got %q", got)
	}
}

// The scan stops at files older than the oldest jot looked for.
func TestOpenURLScanStopsAtJotTime(t *testing.T) {
	dir := t.TempDir()
	svc := sessionService(t)
	svc.SetDesktopSessionsDir(dir)
	desktopFile(t, dir, localA, cliA, false, 3*time.Hour) // older than the jot
	m := map[string]any{"claude_entrypoint": "claude-desktop"}
	if got := openURL(svc, jotMeta(t, svc, "claude", cliA, m)); got != "" {
		t.Fatalf("a file older than the jot was used: %q", got)
	}
}

// One jot's missing stored id doesn't replace another's.
func TestOpenURLKeepsFirstHint(t *testing.T) {
	dir := t.TempDir()
	svc := sessionService(t)
	svc.SetDesktopSessionsDir(dir)
	desktopFile(t, dir, localA, cliA, false, time.Second)
	ep := map[string]any{"claude_entrypoint": "claude-desktop"}
	hinted := map[string]any{"claude_entrypoint": "claude-desktop", "claude_desktop_session_id": localA}
	es := []Entry{jotMeta(t, svc, "claude", cliA, hinted), jotMeta(t, svc, "claude", cliA, ep)}
	refs := []linkRef{refFor(es[:1]), refFor(es[1:])}
	svc.openURLs(refs)
	if _, ok := svc.desktop.misses[cliA]; ok {
		t.Fatal("session recorded as missing")
	}
	if got := svc.openURLs(refs); got[0] == "" || got[1] == "" {
		t.Fatalf("got %q", got)
	}
}

// Without a sessions directory the transcript isn't read at all.
func TestOpenURLNoDirSkipsTranscript(t *testing.T) {
	svc := sessionService(t)
	svc.SetDesktopSessionsDir("")
	tr := filepath.Join(t.TempDir(), "missing.jsonl") // would fail to open if read
	r := refFor([]Entry{jotMeta(t, svc, "claude", cliA, map[string]any{"transcript_path": tr})})
	if got := svc.openURLs([]linkRef{r}); got[0] != "" {
		t.Fatalf("got %q", got[0])
	}
}
