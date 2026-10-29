package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// The authenticator secret is encrypted in the database (AES-256-GCM, key
// derived from SESSION_KEY with a purpose of its own), so a copy of the
// database file alone does not give a second factor away. A value without the
// prefix is a legacy plaintext one and is still read. If SESSION_KEY changes,
// the sealed secret can no longer be read: sign-in then needs the operator's
// MASTER_ADMIN_RESET_2FA.
const sealedPrefix = "enc1:"

func (s *Service) totpAEAD() (cipher.AEAD, error) {
	sum := sha256.Sum256(append([]byte("rafactools two-step secret v1\x00"), s.sessionKey...))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (s *Service) sealSecret(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	aead, err := s.totpAEAD()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return sealedPrefix + base64.StdEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(plain), []byte("totp"))), nil
}

// openSecret returns the plaintext secret; an unreadable one yields "" (so
// the code is refused rather than crashing).
func (s *Service) openSecret(stored string) string {
	if stored == "" || !strings.HasPrefix(stored, sealedPrefix) {
		return stored
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, sealedPrefix))
	aead, aerr := s.totpAEAD()
	if err != nil || aerr != nil || len(raw) < aead.NonceSize() {
		return ""
	}
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], []byte("totp"))
	if err != nil {
		return ""
	}
	return string(plain)
}
