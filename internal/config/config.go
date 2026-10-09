package config

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	RecoveryKey                                                                                                         []byte
	PaymentProvider, MockPaymentSecret                                                                                  string
	Environment, Address, DatabaseURL, FrontendOrigin, MailDirectory, SMTPAddress, SMTPUsername, SMTPPassword, SMTPFrom string
	PanelURL, ClientAPIKey                                                                                              string
	WebsocketOrigins                                                                                                    []string
	TransferOrigins                                                                                                     []string
}

func Load() (Config, error) {
	c := Config{Environment: env("APP_ENV", "development"), Address: env("HTTP_ADDR", ":8080"), DatabaseURL: os.Getenv("DATABASE_URL"), FrontendOrigin: env("FRONTEND_ORIGIN", "http://localhost:3000"), MailDirectory: env("MAIL_DIRECTORY", ".local/mail"), SMTPAddress: os.Getenv("SMTP_ADDRESS"), SMTPUsername: os.Getenv("SMTP_USERNAME"), SMTPPassword: os.Getenv("SMTP_PASSWORD"), SMTPFrom: os.Getenv("SMTP_FROM")}
	rawKey := os.Getenv("RECOVERY_TOKEN_KEY")
	if rawKey != "" {
		var err error
		c.RecoveryKey, err = base64.StdEncoding.DecodeString(rawKey)
		if err != nil || len(c.RecoveryKey) != 32 {
			return c, fmt.Errorf("RECOVERY_TOKEN_KEY must be base64 encoding of 32 random bytes")
		}
	} else if c.Environment == "production" {
		return c, fmt.Errorf("RECOVERY_TOKEN_KEY is required in production")
	}
	c.PanelURL = os.Getenv("PTERODACTYL_URL")
	c.ClientAPIKey = os.Getenv("PTERODACTYL_CLIENT_API_KEY")
	if c.ClientAPIKey != "" && (c.PanelURL == "" || strings.HasPrefix(c.ClientAPIKey, "ptla_")) {
		return c, fmt.Errorf("live management requires a Panel URL and a Client API key")
	}
	for _, raw := range strings.Split(os.Getenv("PTERODACTYL_WEBSOCKET_ORIGINS"), ",") {
		raw = strings.TrimRight(strings.TrimSpace(raw), "/")
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "wss" && !(c.Environment == "development" && u.Scheme == "ws")) {
			return c, fmt.Errorf("invalid allowed Wings WebSocket origin")
		}
		c.WebsocketOrigins = append(c.WebsocketOrigins, raw)
	}

	for _, raw := range strings.Split(os.Getenv("PTERODACTYL_TRANSFER_ORIGINS"), ",") {
		raw = strings.TrimRight(strings.TrimSpace(raw), "/")
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(c.Environment == "development" && u.Scheme == "http")) {
			return c, fmt.Errorf("invalid file transfer origin")
		}
		c.TransferOrigins = append(c.TransferOrigins, raw)
	}
	c.PaymentProvider = env("PAYMENT_PROVIDER", "disabled")
	c.MockPaymentSecret = os.Getenv("MOCK_PAYMENT_SECRET")
	if c.PaymentProvider != "disabled" && c.PaymentProvider != "mock" {
		return c, fmt.Errorf("unsupported payment provider")
	}
	if c.PaymentProvider == "mock" && (c.Environment != "development" || len(c.MockPaymentSecret) < 32 || strings.HasPrefix(c.MockPaymentSecret, "replace-")) {
		return c, fmt.Errorf("mock payments require development and a secret of at least 32 characters")
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	if c.Environment != "development" && c.Environment != "production" {
		return c, fmt.Errorf("APP_ENV must be development or production")
	}
	u, err := url.Parse(c.FrontendOrigin)
	if err != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return c, fmt.Errorf("FRONTEND_ORIGIN must be an exact HTTP origin")
	}
	if c.Environment == "production" {
		if u.Scheme != "https" {
			return c, fmt.Errorf("production requires HTTPS frontend")
		}
		if c.SMTPAddress == "" || c.SMTPFrom == "" {
			return c, fmt.Errorf("production requires SMTP_ADDRESS and SMTP_FROM")
		}
		if err := secureDatabase(c.DatabaseURL); err != nil {
			return c, err
		}
	}
	return c, nil
}
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func secureDatabase(value string) error {
	db, err := url.Parse(value)
	if err != nil || db.Query().Get("sslmode") != "verify-full" {
		return fmt.Errorf("production database requires sslmode=verify-full")
	}
	return nil
}
