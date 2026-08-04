package core

import (
	"golang.org/x/crypto/bcrypt"
)

// HashPassword produces the stored form of a password. Exported so handlers
// can hash a replacement password during an update.
func HashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}
