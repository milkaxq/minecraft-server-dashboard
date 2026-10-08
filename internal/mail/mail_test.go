package mail

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateSpool(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mail")
	if err := (Spool{Directory: dir}).SendVerification(context.Background(), "player@example.com", "secret-token"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("missing delivery", err)
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("mail token file not private")
	}
	body, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil || !strings.Contains(string(body), "secret-token") {
		t.Fatal("token missing", err)
	}
}
