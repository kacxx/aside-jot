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
	t.Setenv("JOT_DB", "/tmp/custom.db")
	if p, _ := DBPath(); p != "/tmp/custom.db" {
		t.Errorf("JOT_DB: %s", p)
	}
	t.Setenv("JOT_DB", "")
	t.Setenv("XDG_DATA_HOME", "/data")
	if p, _ := DBPath(); p != filepath.Join("/data", "jot", "jot.db") {
		t.Errorf("XDG: %s", p)
	}
	if runtime.GOOS == "linux" {
		t.Setenv("XDG_DATA_HOME", "")
		t.Setenv("HOME", "/home/u")
		if p, _ := DBPath(); p != "/home/u/.local/share/jot/jot.db" {
			t.Errorf("default: %s", p)
		}
	}
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
