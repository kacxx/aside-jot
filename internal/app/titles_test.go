package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func titledEntry(source, path string) Entry {
	meta, _ := json.Marshal(map[string]string{"transcript_path": path})
	return Entry{ID: 1, Source: source, Metadata: meta}
}

func titleOf(t *testing.T, source, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	es := []Entry{titledEntry(source, path)}
	(&Service{}).AddSessionTitles(es)
	return es[0].SessionTitle
}

const (
	titleUserLine = `{"type":"user","message":{"content":"hi"}}` + "\n"
	customOne     = `{"type":"custom-title","customTitle":"ESM-6993 review","sessionId":"s"}` + "\n"
	customTwo     = `{"type":"custom-title","customTitle":"ESM-7000 renamed","sessionId":"s"}` + "\n"
	aiTitleOne    = `{"type":"ai-title","aiTitle":"Generated name","sessionId":"s"}` + "\n"
)

func TestSessionTitleCustom(t *testing.T) {
	if got := titleOf(t, "claude", titleUserLine+customOne); got != "ESM-6993 review" {
		t.Errorf("title = %q", got)
	}
}

func TestSessionTitleCustomBeatsNewerAITitle(t *testing.T) {
	if got := titleOf(t, "claude", customOne+aiTitleOne); got != "ESM-6993 review" {
		t.Errorf("title = %q", got)
	}
}

func TestSessionTitleAIFallback(t *testing.T) {
	if got := titleOf(t, "claude", titleUserLine+aiTitleOne); got != "Generated name" {
		t.Errorf("title = %q", got)
	}
}

func TestSessionTitleRenamedNewestWins(t *testing.T) {
	if got := titleOf(t, "claude", customOne+titleUserLine+customTwo); got != "ESM-7000 renamed" {
		t.Errorf("title = %q", got)
	}
}

func TestSessionTitleNone(t *testing.T) {
	if got := titleOf(t, "claude", titleUserLine); got != "" {
		t.Errorf("title = %q", got)
	}
}

func TestSessionTitleMissingTranscript(t *testing.T) {
	es := []Entry{titledEntry("claude", filepath.Join(t.TempDir(), "gone.jsonl"))}
	(&Service{}).AddSessionTitles(es)
	if es[0].SessionTitle != "" {
		t.Errorf("title = %q", es[0].SessionTitle)
	}
}

func TestSessionTitleOnlyForClaude(t *testing.T) {
	for _, source := range []string{"codex", "cursor", "cli", ""} {
		if got := titleOf(t, source, customOne); got != "" {
			t.Errorf("source %q: title = %q", source, got)
		}
	}
}

func TestSessionTitleReadsOnlyTheTail(t *testing.T) {
	// An old title beyond the tail window is not read; a cut first line is skipped.
	filler := `{"type":"user","message":{"content":"` + strings.Repeat("x", 1000) + `"}}` + "\n"
	content := customOne + strings.Repeat(filler, titleTailBytes/len(filler)+2)
	if got := titleOf(t, "claude", content); got != "" {
		t.Errorf("title = %q, want none from outside the tail", got)
	}
	if got := titleOf(t, "claude", content+customTwo); got != "ESM-7000 renamed" {
		t.Errorf("title = %q", got)
	}
}
