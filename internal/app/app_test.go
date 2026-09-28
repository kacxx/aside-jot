package app

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/kacxx/aside-jot/internal/gitctx"
)

func TestDBPath(t *testing.T) {
	dir := t.TempDir() // a native absolute path on every OS
	check := func(what, want string) {
		t.Helper()
		got, err := DBPath()
		if err != nil || got != want {
			t.Errorf("%s: got %q (%v), want %q", what, got, err, want)
		}
	}

	custom := filepath.Join(dir, "custom.db")
	t.Setenv("JOT_DB", custom)
	check("JOT_DB", custom)

	t.Setenv("JOT_DB", "rel.db")
	if p, err := DBPath(); err != nil || !filepath.IsAbs(p) || filepath.Base(p) != "rel.db" {
		t.Errorf("relative JOT_DB must be made absolute: %q (%v)", p, err)
	}

	t.Setenv("JOT_DB", "")
	t.Setenv("XDG_DATA_HOME", dir)
	check("XDG_DATA_HOME", filepath.Join(dir, "jot", "jot.db"))

	// The per-OS default, which a relative XDG_DATA_HOME also falls back to.
	var want string
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LOCALAPPDATA", dir)
		want = filepath.Join(dir, "jot", "jot.db")
	case "darwin":
		t.Setenv("HOME", dir)
		want = filepath.Join(dir, "Library", "Application Support", "jot", "jot.db")
	default:
		t.Setenv("HOME", dir)
		want = filepath.Join(dir, ".local", "share", "jot", "jot.db")
	}
	t.Setenv("XDG_DATA_HOME", "")
	check("default", want)
	t.Setenv("XDG_DATA_HOME", "relative")
	check("relative XDG_DATA_HOME", want)
}

func TestBusyTimeout(t *testing.T) {
	t.Setenv("JOT_BUSY_TIMEOUT_MS", "")
	if BusyTimeout() != 2*time.Second {
		t.Error("default")
	}
	t.Setenv("JOT_BUSY_TIMEOUT_MS", "150")
	if BusyTimeout() != 150*time.Millisecond {
		t.Error("override")
	}
	t.Setenv("JOT_BUSY_TIMEOUT_MS", "junk")
	if BusyTimeout() != 2*time.Second {
		t.Error("invalid falls back")
	}
	for _, v := range []string{"10001", "65000", "99999999999999999"} {
		t.Setenv("JOT_BUSY_TIMEOUT_MS", v)
		if got := BusyTimeout(); got != MaxBusyTimeout {
			t.Errorf("%s: got %v, want the %v cap", v, got, MaxBusyTimeout)
		}
	}
	t.Setenv("JOT_BUSY_TIMEOUT_MS", "999999999999999999999") // overflows int64
	if BusyTimeout() != DefaultBusyTimeout {
		t.Error("unparseable falls back")
	}
}

func TestServiceFlow(t *testing.T) {
	ctx := context.Background()
	svc, err := Open(filepath.Join(t.TempDir(), "jot.db"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	svc.git = func(context.Context, string) gitctx.Info {
		return gitctx.Info{Root: "/r", Name: "r", Branch: "b", Commit: "c"}
	}

	e, err := svc.Capture(ctx, CaptureRequest{Text: "  hello  ", Source: "cli", Cwd: "/r",
		Metadata: map[string]any{"x": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if e.ID != 1 || e.Text != "hello" || e.Branch != "b" || string(e.Metadata) != `{"x":1}` {
		t.Fatalf("capture: %+v", e)
	}
	if _, err := svc.Capture(ctx, CaptureRequest{Text: "  "}); err == nil {
		t.Fatal("empty capture should fail")
	}
	if got, _ := svc.Search(ctx, "HELL", 0); len(got) != 1 {
		t.Fatal("search")
	}
	if _, err := svc.Search(ctx, " ", 0); err == nil {
		t.Fatal("empty search should fail")
	}
	if err := svc.Done(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if in, _ := svc.Inbox(ctx, 10); len(in) != 0 {
		t.Fatal("inbox should be empty")
	}
	if got, _ := svc.Show(ctx, 1); got.Status != "done" {
		t.Fatalf("show: %+v", got)
	}
}
