package db

import (
	"path/filepath"
	"testing"
)

func TestOpenAppliesMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	database, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	for _, table := range []string{"squadrons", "users", "programmes", "programme_days", "programme_rows", "invites"} {
		var name string
		err := database.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q to exist after migration: %v", table, err)
		}
	}

	// Re-opening (i.e. restarting the app) must not re-apply migrations or error.
	database2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer database2.Close()

	var count int
	if err := database2.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count != 2 {
		t.Errorf("schema_migrations has %d rows, want 2 (a migration re-applied, or a new one needs counting here)", count)
	}
}

func TestForeignKeysAndConstraintsAreEnforced(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	// Orphan rows must be rejected: foreign_keys pragma has to be on for
	// every pooled connection (the app relies on ON DELETE CASCADE).
	if _, err := d.Exec(`INSERT INTO users (squadron_id, email, password_hash) VALUES (999, 'o@x', 'h')`); err == nil {
		t.Error("user with a nonexistent squadron accepted: foreign keys are off")
	}
	d.Exec(`INSERT INTO squadrons (number, name) VALUES ('1', 'One')`)
	if _, err := d.Exec(`INSERT INTO squadrons (number, name) VALUES ('1', 'Dup')`); err == nil {
		t.Error("duplicate squadron number accepted")
	}
	if _, err := d.Exec(`INSERT INTO users (squadron_id, email, password_hash, role) VALUES (1, 'a@x', 'h', 'root')`); err == nil {
		t.Error("invalid role accepted: CHECK constraint missing")
	}
	d.Exec(`INSERT INTO users (squadron_id, email, password_hash, role) VALUES (1, 'a@x', 'h', 'owner')`)
	if _, err := d.Exec(`INSERT INTO users (squadron_id, email, password_hash, role) VALUES (1, 'a@x', 'h', 'viewer')`); err == nil {
		t.Error("duplicate email accepted")
	}
	d.Exec(`DELETE FROM squadrons WHERE id = 1`)
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	if n != 0 {
		t.Errorf("%d users survived their squadron's deletion", n)
	}
}

func TestOpenUsesWAL(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "w.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var mode string
	d.QueryRow(`PRAGMA journal_mode`).Scan(&mode)
	if mode != "wal" {
		t.Errorf("journal_mode = %q", mode)
	}
}

func TestOpenFailsCleanlyOnUnwritablePath(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "no", "such", "dir", "x.db")); err == nil {
		t.Error("Open succeeded for a path whose directory does not exist")
	}
}
