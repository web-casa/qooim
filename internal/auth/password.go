package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// ErrBadCredentials is returned by Verify when the password does not match.
// Callers should treat this as a 401 — never echo why.
var ErrBadCredentials = errors.New("invalid credentials")

// VerifyPassword compares a bcrypt hash (SK uses $2a$10$...) with a plaintext
// password. Errors other than mismatch (malformed hash) are returned as-is so
// the caller can distinguish data corruption from a bad login attempt.
func VerifyPassword(hash, plain string) error {
	if hash == "" || plain == "" {
		return ErrBadCredentials
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return ErrBadCredentials
	}
	return err
}

// HashPassword wraps bcrypt.GenerateFromPassword with the cost SK used (10).
// Reserved for the password-change flow in P2+.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), 10)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
