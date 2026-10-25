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
	if count != 1 {
		t.Errorf("schema_migrations has %d rows, want 1 (migration re-applied?)", count)
	}
}
