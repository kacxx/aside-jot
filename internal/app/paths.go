package app

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

// DefaultBusyTimeout is used when $JOT_BUSY_TIMEOUT_MS is unset or invalid.
const DefaultBusyTimeout = 2000 * time.Millisecond

// MaxBusyTimeout caps $JOT_BUSY_TIMEOUT_MS. A capture waits up to twice this
// (open, then insert) plus git lookup; the cap keeps that well inside Claude
// Code's 30 s UserPromptSubmit hook timeout, since a hook the host kills can
// no longer block the jot.
const MaxBusyTimeout = 10 * time.Second

// DBPath resolves the database path: $JOT_DB, else $XDG_DATA_HOME/jot/jot.db,
// else the OS per-user data directory.
func DBPath() (string, error) {
	if p := os.Getenv("JOT_DB"); p != "" {
		return filepath.Abs(p)
	}
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "jot.db"), nil
}

// DataDir is the directory jot stores its database in when $JOT_DB is unset.
func DataDir() (string, error) {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" && filepath.IsAbs(x) {
		return filepath.Join(x, "jot"), nil
	}
	switch runtime.GOOS {
	case "windows":
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, "jot"), nil
		}
		return "", errors.New("cannot locate data dir: %LOCALAPPDATA% is not set")
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "jot"), nil
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "share", "jot"), nil
	}
}

// BusyTimeout reads $JOT_BUSY_TIMEOUT_MS, defaulting to 2000ms and capped at
// MaxBusyTimeout.
func BusyTimeout() time.Duration {
	if v := os.Getenv("JOT_BUSY_TIMEOUT_MS"); v != "" {
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil && ms >= 0 {
			if ms > MaxBusyTimeout.Milliseconds() {
				return MaxBusyTimeout
			}
			return time.Duration(ms) * time.Millisecond
		}
	}
	return DefaultBusyTimeout
}
