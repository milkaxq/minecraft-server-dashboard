// Package mail delivers verification tokens without writing secrets to application logs.
package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Spool is local development delivery, not an email provider. Files are private to the process owner.
type Spool struct{ Directory string }

func (s Spool) SendVerification(ctx context.Context, email, token string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(s.Directory, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.Directory, uuid.NewString()+".txt"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "To: %s\nVerification token (expires in 24 hours): %s\nPOST /auth/verify-email with {\"token\":\"<token>\"}\n", email, token)
	return err
}

// SMTP requires implicit TLS (usually port 465); plaintext/opportunistic TLS is not allowed.
type SMTP struct{ Address, Username, Password, From string }

func (s SMTP) SendVerification(ctx context.Context, email, token string) error {
	if strings.ContainsAny(s.From+email, "\r\n") {
		return fmt.Errorf("invalid mail address")
	}
	host, _, err := net.SplitHostPort(s.Address)
	if err != nil {
		return err
	}
	dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: 10 * time.Second}, Config: &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}}
	conn, err := dialer.DialContext(ctx, "tcp", s.Address)
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline := time.Now().Add(15 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err = conn.SetDeadline(deadline); err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if s.Username != "" {
		if err = client.Auth(smtp.PlainAuth("", s.Username, s.Password, host)); err != nil {
			return err
		}
	}
	if err = client.Mail(s.From); err != nil {
		return err
	}
	if err = client.Rcpt(email); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "From: %s\r\nTo: %s\r\nSubject: Verify your Minecraft hosting account\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nYour verification token (valid for 24 hours): %s\r\n", s.From, email, token)
	if err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
