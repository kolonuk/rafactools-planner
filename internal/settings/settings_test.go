package settings

import (
	"path/filepath"
	"strings"
	"testing"

	"rafactools-planner/internal/db"
)

func newStore(t *testing.T, key string, path string) *Store {
	t.Helper()
	d, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s, err := New(d, key, []string{"A", "B", "SECRET"}, []string{"SECRET"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

const key1 = "session-key-one-0123456789abcdef0123"

func TestSetGetDeleteAndUnknownKeys(t *testing.T) {
	s := newStore(t, key1, filepath.Join(t.TempDir(), "s.db"))
	if err := s.Set("A", "value", "me"); err != nil {
		t.Fatal(err)
	}
	all, err := s.All()
	if err != nil || all["A"] != "value" {
		t.Fatalf("All = %v, %v", all, err)
	}
	if err := s.Set("A", "", "me"); err != nil { // empty removes
		t.Fatal(err)
	}
	if all, _ := s.All(); len(all) != 0 {
		t.Errorf("empty value did not remove: %v", all)
	}
	if err := s.Set("NOT_ALLOWED", "x", "me"); err != ErrUnknownKey {
		t.Errorf("unknown key = %v", err)
	}
	if err := s.Delete("NOT_ALLOWED"); err != ErrUnknownKey {
		t.Errorf("delete unknown key = %v", err)
	}
}

func TestSecretsAreEncryptedAtRestAndNeedTheSameKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.db")
	s := newStore(t, key1, path)
	if err := s.Set("SECRET", "hunter2-super-secret", "me"); err != nil {
		t.Fatal(err)
	}
	var raw string
	s.db.QueryRow(`SELECT value FROM site_settings WHERE key = 'SECRET'`).Scan(&raw)
	if raw == "" || strings.Contains(raw, "hunter2") {
		t.Fatalf("secret stored in the clear: %q", raw)
	}
	if all, err := s.All(); err != nil || all["SECRET"] != "hunter2-super-secret" {
		t.Fatalf("round trip: %v %v", all, err)
	}

	// Two saves of the same secret differ (fresh nonce each time).
	s.Set("SECRET", "hunter2-super-secret", "me")
	var raw2 string
	s.db.QueryRow(`SELECT value FROM site_settings WHERE key = 'SECRET'`).Scan(&raw2)
	if raw == raw2 {
		t.Error("the same ciphertext was written twice (nonce reuse)")
	}

	// A different SESSION_KEY cannot read it: the secret is skipped and named,
	// the other settings still load.
	s.Set("A", "plain", "me")
	other, _ := New(s.db, "a-completely-different-session-key-0123", []string{"A", "B", "SECRET"}, []string{"SECRET"})
	all, err := other.All()
	if err == nil || !strings.Contains(err.Error(), "SECRET") {
		t.Errorf("wrong key err = %v, want it to name SECRET", err)
	}
	if _, ok := all["SECRET"]; ok || all["A"] != "plain" {
		t.Errorf("wrong key result = %v", all)
	}
}

func TestASealedValueCannotBeMovedToAnotherSetting(t *testing.T) {
	s := newStore(t, key1, filepath.Join(t.TempDir(), "s.db"))
	s.Set("SECRET", "value", "me")
	s.db.Exec(`UPDATE site_settings SET key = 'B' WHERE key = 'SECRET'`)
	s.db.Exec(`UPDATE site_settings SET is_secret = 1 WHERE key = 'B'`)
	s2, _ := New(s.db, key1, []string{"A", "B", "SECRET"}, []string{"SECRET", "B"})
	if all, err := s2.All(); err == nil || all["B"] != "" {
		t.Errorf("a sealed value was accepted under another name: %v %v", all, err)
	}
}

func TestGetterPrefersTheDatabaseThenTheEnvironment(t *testing.T) {
	s := newStore(t, key1, filepath.Join(t.TempDir(), "s.db"))
	env := map[string]string{"A": "env-a", "B": "env-b"}
	get, err := s.Getter(func(k string) string { return env[k] })
	if err != nil || get("A") != "env-a" || get("B") != "env-b" {
		t.Fatalf("env only: %v", err)
	}
	s.Set("A", "db-a", "me")
	get, _ = s.Getter(func(k string) string { return env[k] })
	if get("A") != "db-a" || get("B") != "env-b" {
		t.Errorf("A=%q B=%q, want the database to win only for A", get("A"), get("B"))
	}
	if s.Source("A", func(k string) string { return env[k] }) != "database" || s.Source("B", func(k string) string { return env[k] }) != "environment" || s.Source("SECRET", func(string) string { return "" }) != "default" {
		t.Error("Source labels wrong")
	}
}

func TestNewRejectsSecretOutsideAllowedList(t *testing.T) {
	d, _ := db.Open(filepath.Join(t.TempDir(), "s.db"))
	defer d.Close()
	if _, err := New(d, key1, []string{"A"}, []string{"B"}); err == nil {
		t.Error("a secret key that is not allowed was accepted")
	}
}
