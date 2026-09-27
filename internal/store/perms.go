package store

import (
	"errors"
	"fmt"
	"io/fs"
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
// It refuses a database or sidecar that is a symlink, not a regular file, or
// owned by another user: in a shared directory such as /tmp, a file planted
// there in advance would otherwise receive the user's jots. Tightening the
// mode stays best effort. Windows does not use Unix permission bits.
func secureDBFiles(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	// O_EXCL also refuses to create through a symlink at path.
	if f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, filePerm); err == nil {
		_ = f.Close()
	}
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		fi, err := os.Lstat(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("refusing to use %s: not a regular file", p)
		}
		if !ownedByCurrentUser(fi) {
			return fmt.Errorf("refusing to use %s: owned by another user", p)
		}
		if fi.Mode().Perm()&0o077 != 0 {
			_ = os.Chmod(p, fi.Mode().Perm()&^0o077)
		}
	}
	return nil
}
