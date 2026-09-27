package store

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func mode(t *testing.T, p string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

func TestFilesAreOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	// A permissive umask must not leak into the database or backups.
	old := setUmask(0o022)
	defer setUmask(old)

	dir := t.TempDir()
	path := filepath.Join(dir, "jot.db")
	s, err := Open(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Insert(context.Background(), &Entry{Text: "secret"}); err != nil {
		t.Fatal(err)
	}
	// The WAL and shared-memory files exist while a connection is open.
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if m := mode(t, p); m != 0o600 {
			t.Errorf("%s: mode %o, want 600", filepath.Base(p), m)
		}
	}

	dst := filepath.Join(dir, "backup.db")
	if err := s.Backup(context.Background(), dst); err != nil {
		t.Fatal(err)
	}
	if m := mode(t, dst); m != 0o600 {
		t.Errorf("backup: mode %o, want 600", m)
	}
}

func TestExistingDatabaseIsTightened(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	path := filepath.Join(t.TempDir(), "jot.db")
	s, err := Open(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if m := mode(t, path); m != 0o600 {
		t.Fatalf("mode %o, want 600", m)
	}
}

func TestFailedBackupLeavesNoFile(t *testing.T) {
	s, _ := openTemp(t)
	s.Close() // make VACUUM INTO fail
	dst := filepath.Join(t.TempDir(), "backup.db")
	if err := s.Backup(context.Background(), dst); err == nil {
		t.Fatal("expected backup on a closed store to fail")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("failed backup left %s behind (%v)", dst, err)
	}
}
