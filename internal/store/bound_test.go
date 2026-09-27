package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// TestOpenHonoursBusyTimeout checks that Open waits at most its busy timeout
// in total. The WAL-switch race that makes early attempts fail fast can't be
// triggered on demand, so attempts made in the first 3/4 of the budget get a
// zero timeout (a real, immediate SQLITE_BUSY against a locked database); later
// attempts use the timeout Open hands them. Passing the full busy timeout to
// every attempt would take about 1.75x the budget here.
func TestOpenHonoursBusyTimeout(t *testing.T) {
	const budget = 400 * time.Millisecond
	path := filepath.Join(t.TempDir(), "jot.db")

	// A WAL file without the schema, write-locked by another connection.
	other, err := sql.Open("sqlite", DSN(path, 0))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	ctx := context.Background()
	conn, err := other.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, q := range []string{"CREATE TABLE placeholder(x)", "BEGIN EXCLUSIVE"} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	defer conn.ExecContext(ctx, "ROLLBACK")

	start := time.Now()
	var attempts int
	var lastTimeout time.Duration
	openAttempt = func(p string, timeout time.Duration) error {
		attempts++
		lastTimeout = timeout
		if time.Since(start) < budget*3/4 {
			timeout = 0
		}
		return migrateWithTimeout(p, timeout)
	}
	t.Cleanup(func() { openAttempt = migrateWithTimeout })

	s, err := Open(path, budget)
	elapsed := time.Since(start)
	if err == nil {
		s.Close()
		t.Fatal("expected Open to fail while the database stays locked")
	}
	if !isBusy(err) {
		t.Fatalf("expected a busy error, got %v", err)
	}
	if limit := budget * 12 / 10; elapsed > limit {
		t.Fatalf("Open took %v with a %v budget (limit %v)", elapsed, budget, limit)
	}
	if attempts < 2 || lastTimeout >= budget {
		t.Fatalf("attempts=%d lastTimeout=%v: later attempts must get the remaining budget", attempts, lastTimeout)
	}
	t.Logf("Open failed after %v over %d attempts: %v", elapsed, attempts, err)
}
