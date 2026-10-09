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

func TestResetSpoolUsesTrustedOriginAndFragment(t *testing.T) {
	dir := t.TempDir()
	if err := (Spool{Directory: dir, FrontendOrigin: "https://hosting.example.com"}).SendPasswordReset(context.Background(), "player@example.com", "test-token"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal(err)
	}
	info, err := entries[0].Info()
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("insecure spool", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "https://hosting.example.com/reset-password#token=test-token") || !strings.Contains(string(body), "30 minutes") {
		t.Fatal("reset instructions missing")
	}
	if err := (Spool{Directory: dir}).SendPasswordReset(context.Background(), "player@example.com\r\nBcc: other@example.com", "token"); err == nil {
		t.Fatal("mail injection accepted")
	}
}
