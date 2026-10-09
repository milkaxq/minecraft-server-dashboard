package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestProductionConfiguration(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("RECOVERY_TOKEN_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	t.Setenv("DATABASE_URL", "postgres://user:password@db/hosting?sslmode=disable")
	t.Setenv("FRONTEND_ORIGIN", "https://hosting.example.com")
	t.Setenv("SMTP_ADDRESS", "smtp.example.com:465")
	t.Setenv("SMTP_FROM", "accounts@example.com")
	if _, err := Load(); err == nil {
		t.Fatal("insecure production database accepted")
	}
	t.Setenv("DATABASE_URL", "postgres://user:password@db/hosting?sslmode=verify-full")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FRONTEND_ORIGIN", "http://hosting.example.com")
	if _, err := Load(); err == nil {
		t.Fatal("insecure production origin accepted")
	}
}

func TestTransferOriginsRejectCredentialsPathsAndInsecureProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("RECOVERY_TOKEN_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	t.Setenv("DATABASE_URL", "postgres://user:password@db/hosting?sslmode=verify-full")
	t.Setenv("FRONTEND_ORIGIN", "https://hosting.example.com")
	t.Setenv("SMTP_ADDRESS", "smtp.example.com:465")
	t.Setenv("SMTP_FROM", "accounts@example.com")
	for _, value := range []string{"http://wings.example.com", "https://user:pass@wings.example.com", "https://wings.example.com/files", "https://wings.example.com?token=private", "https://wings.example.com#fragment", "file:///etc/passwd"} {
		t.Setenv("PTERODACTYL_TRANSFER_ORIGINS", value)
		if _, err := Load(); err == nil {
			t.Errorf("unsafe transfer origin accepted: %s", value)
		}
	}
	t.Setenv("PTERODACTYL_TRANSFER_ORIGINS", "https://wings.example.com:8080,https://bucket.s3.example.com")
	c, err := Load()
	if err != nil || len(c.TransferOrigins) != 2 {
		t.Fatal(c.TransferOrigins, err)
	}
}

func TestMockPaymentsCannotRunInProductionOrWithPlaceholderSecrets(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:password@db/hosting?sslmode=disable")
	t.Setenv("APP_ENV", "development")
	t.Setenv("PAYMENT_PROVIDER", "mock")
	t.Setenv("MOCK_PAYMENT_SECRET", "replace-with-a-secret-long-enough")
	if _, err := Load(); err == nil {
		t.Fatal("placeholder payment secret accepted")
	}
	t.Setenv("MOCK_PAYMENT_SECRET", "test-only-payment-secret-at-least-32-characters")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_ENV", "production")
	t.Setenv("RECOVERY_TOKEN_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	if _, err := Load(); err == nil {
		t.Fatal("mock payments accepted in production")
	}
}

func TestRecoveryKeyValidation(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://db/hosting?sslmode=disable")
	for _, value := range []string{"not-base64", base64.StdEncoding.EncodeToString([]byte("short"))} {
		t.Setenv("RECOVERY_TOKEN_KEY", value)
		if _, err := Load(); err == nil {
			t.Fatal("invalid key accepted")
		}
	}
	t.Setenv("RECOVERY_TOKEN_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	if c, err := Load(); err != nil || len(c.RecoveryKey) != 32 {
		t.Fatal(err)
	}
	t.Setenv("APP_ENV", "production")
	t.Setenv("RECOVERY_TOKEN_KEY", "")
	if _, err := Load(); err == nil {
		t.Fatal("production recovery enabled without key")
	}
}
