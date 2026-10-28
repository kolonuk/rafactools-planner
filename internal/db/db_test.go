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

	// Count applied migrations after the first Open — not hardcoded, since
	// the real number grows every time a migration file is added (it's 2 as
	// of writing: 0001_init.sql, 0002_squadron_website.sql).
	var firstCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&firstCount); err != nil {
		t.Fatalf("count schema_migrations after first Open: %v", err)
	}
	if firstCount == 0 {
		t.Fatal("no migrations recorded after first Open")
	}

	// Re-opening (i.e. restarting the app) must not re-apply migrations or error.
	database2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer database2.Close()

	var secondCount int
	if err := database2.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&secondCount); err != nil {
		t.Fatalf("count schema_migrations after second Open: %v", err)
	}
	if secondCount != firstCount {
		t.Errorf("schema_migrations had %d rows after first Open, %d after second — migration re-applied?", firstCount, secondCount)
	}
}
