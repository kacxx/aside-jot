package setup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// ConfigPath is the agent's hook config: ~/.claude/settings.json (under
// $CLAUDE_CONFIG_DIR if set) or ~/.codex/hooks.json (under $CODEX_HOME).
func ConfigPath(agent string) (string, error) {
	var env, dir, file string
	switch agent {
	case "claude":
		env, dir, file = "CLAUDE_CONFIG_DIR", ".claude", "settings.json"
	case "codex":
		env, dir, file = "CODEX_HOME", ".codex", "hooks.json"
	default:
		return "", fmt.Errorf("unknown agent %q", agent)
	}
	if d := os.Getenv(env); d != "" {
		return filepath.Join(d, file), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, dir, file), nil
}

// Result describes an Install.
type Result struct {
	Path   string
	Change Change
	Backup string // the backup written before the change, "" if none was needed
	New    []byte // the resulting file contents
	Wrote  bool
}

// Install merges h into the config at path. Unless dryRun, a changed file is
// backed up to <path>.bak-<timestamp> first and then replaced. A file that
// can't be parsed is left untouched and named in the error.
func Install(path string, h Hook, dryRun bool, now time.Time) (Result, error) {
	res := Result{Path: path}
	old, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return res, err
	}
	out, ch, err := Merge(old, h)
	if err != nil {
		return res, fmt.Errorf("%s: %w; left untouched", path, err)
	}
	res.Change, res.New = ch, out
	if ch.Kind == "unchanged" || dryRun {
		return res, nil
	}

	// Write through a symlink (dotfile managers link these files) instead of
	// replacing it, but keep the backup next to the path as the user knows it.
	target := path
	if r, err := filepath.EvalSymlinks(path); err == nil {
		target = r
	}
	mode := fs.FileMode(0o600)
	if fi, err := os.Stat(target); err == nil {
		mode = fi.Mode().Perm()
	}
	if exists {
		if res.Backup, err = writeBackup(path, old, mode, now); err != nil {
			return res, fmt.Errorf("backing up %s: %w", path, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return res, err
	}
	if err := writeAtomic(target, out, mode); err != nil {
		return res, err
	}
	res.Wrote = true
	return res, nil
}

// writeBackup writes data to <path>.bak-<timestamp>, never overwriting.
func writeBackup(path string, data []byte, mode fs.FileMode, now time.Time) (string, error) {
	base := path + ".bak-" + now.Format("20060102-150405")
	for i := 0; i < 100; i++ {
		name := base
		if i > 0 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			return "", err
		}
		return name, f.Close()
	}
	return "", errors.New("too many backups with the same timestamp")
}

// writeAtomic replaces path via a temporary file in the same directory, so a
// crash can't leave a half-written config.
func writeAtomic(path string, data []byte, mode fs.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
