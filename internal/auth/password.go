package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

var ErrInvalid = errors.New("invalid input")
var ErrCredentials = errors.New("invalid credentials")
var ErrUnverified = errors.New("email verification required")
var ErrDuplicate = errors.New("email already registered")
var ErrRateLimited = errors.New("too many requests")

func HashPassword(password string) (string, error) {
	if len(password) < 12 || len(password) > 72 {
		return "", ErrInvalid
	}
	v, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(v), err
}
func CheckPassword(hash, password string) bool {
	return len(password) <= 72 && bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
func newToken() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, tokenHash(token), nil
}
func tokenHash(token string) []byte { sum := sha256.Sum256([]byte(token)); return sum[:] }
