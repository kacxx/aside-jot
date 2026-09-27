package store

import (
	"os"
	"runtime"
)

// filePerm is the mode for the database, its -wal and -shm files, and backups.
// Jots hold whatever the user types, so only the owner may read them.
const filePerm = 0o600

// secureDBFiles makes the database at path owner-only before SQLite opens it.
// A missing database is created empty with filePerm, and SQLite gives the -wal
// and -shm files the same mode as the database. An existing database and its
// sidecar files have their group and other bits removed.
//
// It is best effort: a capture must not fail because, say, $JOT_DB points at a
// file jot may not chmod. Windows does not use Unix permission bits.
func secureDBFiles(path string) {
	if runtime.GOOS == "windows" {
		return
	}
	if f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, filePerm); err == nil {
		f.Close()
	}
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if fi, err := os.Stat(p); err == nil && fi.Mode().Perm()&0o077 != 0 {
			_ = os.Chmod(p, fi.Mode().Perm()&^0o077)
		}
	}
}
