// Package settings holds site settings the master admin can change from the
// website, stored in the database and layered over the environment: a value
// saved here wins, anything not saved falls back to the environment variable
// of the same name. Secrets are encrypted at rest with a key derived from the
// site's SESSION_KEY, so a copy of the database file alone does not reveal
// them.
package settings

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"time"
)

// ErrUnknownKey is returned for a key that is not in the allowed list.
var ErrUnknownKey = errors.New("settings: unknown key")

// Store reads and writes settings. Only the keys it was created with can be
// stored; the secret ones are encrypted.
type Store struct {
	db      *sql.DB
	aead    cipher.AEAD
	allowed map[string]bool // key -> is secret
}

// New creates a Store. allowed lists the keys that may be stored, and secret
// which of them are encrypted.
func New(db *sql.DB, sessionKey string, allowed, secret []string) (*Store, error) {
	// A key of its own, so the signing key is never used for two jobs.
	sum := sha256.Sum256([]byte("rafactools settings encryption v1\x00" + sessionKey))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, aead: aead, allowed: map[string]bool{}}
	for _, k := range allowed {
		s.allowed[k] = false
	}
	for _, k := range secret {
		if _, ok := s.allowed[k]; !ok {
			return nil, fmt.Errorf("settings: secret key %q is not in the allowed list", k)
		}
		s.allowed[k] = true
	}
	return s, nil
}

func (s *Store) seal(key, plain string) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	// The key name is authenticated too, so a sealed value cannot be moved to
	// another setting.
	out := s.aead.Seal(nonce, nonce, []byte(plain), []byte(key))
	return base64.StdEncoding.EncodeToString(out), nil
}

func (s *Store) open(key, sealed string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil || len(raw) < s.aead.NonceSize() {
		return "", errors.New("settings: stored secret is malformed")
	}
	nonce, ct := raw[:s.aead.NonceSize()], raw[s.aead.NonceSize():]
	plain, err := s.aead.Open(nil, nonce, ct, []byte(key))
	if err != nil {
		return "", errors.New("settings: stored secret cannot be decrypted (was SESSION_KEY changed?)")
	}
	return string(plain), nil
}

// Set stores a value; an empty value removes the setting (so the environment's
// default applies again).
func (s *Store) Set(key, value, by string) error {
	secret, ok := s.allowed[key]
	if !ok {
		return ErrUnknownKey
	}
	if value == "" {
		return s.Delete(key)
	}
	stored := value
	if secret {
		var err error
		if stored, err = s.seal(key, value); err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`INSERT INTO site_settings (key, value, is_secret, updated_at, updated_by) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, is_secret = excluded.is_secret, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
		key, stored, boolInt(secret), time.Now().UTC(), by)
	return err
}

// Delete removes a setting.
func (s *Store) Delete(key string) error {
	if _, ok := s.allowed[key]; !ok {
		return ErrUnknownKey
	}
	_, err := s.db.Exec(`DELETE FROM site_settings WHERE key = ?`, key)
	return err
}

// All returns every stored setting, secrets decrypted. A secret that cannot be
// decrypted is left out (and named in the error) rather than failing the rest,
// so the environment's default is used for it.
func (s *Store) All() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value, is_secret FROM site_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	var bad []string
	for rows.Next() {
		var k, v string
		var secret int
		if err := rows.Scan(&k, &v, &secret); err != nil {
			return nil, err
		}
		if _, ok := s.allowed[k]; !ok {
			continue
		}
		if secret == 1 {
			plain, err := s.open(k, v)
			if err != nil {
				bad = append(bad, k)
				continue
			}
			v = plain
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return out, fmt.Errorf("settings: could not decrypt %v (was SESSION_KEY changed? set them again)", bad)
	}
	return out, nil
}

// Getter returns a lookup that prefers a stored setting and falls back to
// env. It reads the database each call it is built for, so build it when
// settings are needed, not once at startup.
func (s *Store) Getter(env func(string) string) (func(string) string, error) {
	all, err := s.All()
	return func(k string) string {
		if v, ok := all[k]; ok && v != "" {
			return v
		}
		return env(k)
	}, err
}

// Source says where a setting's effective value comes from.
func (s *Store) Source(key string, env func(string) string) string {
	all, _ := s.All()
	switch {
	case all[key] != "":
		return "database"
	case env(key) != "":
		return "environment"
	}
	return "default"
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
