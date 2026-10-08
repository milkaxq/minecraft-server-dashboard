package auth

import (
	"bytes"
	"strings"
	"testing"
)

func TestPasswords(t *testing.T) {
	h, err := HashPassword("a-long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "a-long-test-password") || CheckPassword(h, "incorrect") || CheckPassword("invalid", "a-long-test-password") {
		t.Fatal("password verification failed")
	}
	h2, err := HashPassword("a-long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if h == h2 {
		t.Fatal("missing random salt")
	}
	for _, p := range []string{"short", strings.Repeat("a", 73)} {
		if _, err := HashPassword(p); err == nil {
			t.Fatal("accepted invalid length")
		}
	}
}
func TestTokens(t *testing.T) {
	a, ha, err := newToken()
	if err != nil {
		t.Fatal(err)
	}
	b, hb, err := newToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 43 || a == b || len(ha) != 32 || bytes.Equal(ha, hb) || !bytes.Equal(ha, tokenHash(a)) {
		t.Fatal("token generation failed")
	}
}
