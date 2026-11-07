package auth

import (
	"os"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// Tests sign in constantly; the cheapest bcrypt cost keeps them fast (see HashCost).
func TestMain(m *testing.M) {
	HashCost = bcrypt.MinCost
	os.Exit(m.Run())
}
