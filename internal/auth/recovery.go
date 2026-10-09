package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrResetToken = errors.New("reset token is invalid or expired")
var ErrCurrentPassword = errors.New("current password is incorrect")
var ErrRecoveryUnavailable = errors.New("account recovery is not configured")

// RequestPasswordReset queues mail, never waiting on SMTP in a public request.
// Unknown/unverified accounts and account-level throttles have the same response.
func (s *Service) RequestPasswordReset(ctx context.Context, email, rid string) error {
	if len(s.RecoveryKey) != 32 {
		return ErrRecoveryUnavailable
	}
	email, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	if err = s.RateLimit(ctx, fmt.Sprintf("recovery:%x", tokenHash(email)), 5); errors.Is(err, ErrRateLimited) {
		return nil
	} else if err != nil {
		return err
	}
	token, digest, err := newToken()
	if err != nil {
		return err
	}
	encrypted, err := s.encryptReset(token, digest)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var id uuid.UUID
	err = tx.QueryRow(ctx, "SELECT id FROM users WHERE email=$1 AND verified_at IS NOT NULL FOR UPDATE", email).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO password_resets(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '30 minutes')", digest, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO recovery_mail(id,token_hash,encrypted_token,request_id) VALUES($1,$2,$3,$4)", uuid.New(), digest, encrypted, rid); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_logs(id,actor_id,action,request_id) VALUES($1,$2,'auth.reset_requested',$3)", uuid.New(), id, rid); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ResetPassword(ctx context.Context, token, password, rid string) error {
	if len(token) != 43 {
		return ErrResetToken
	}
	// Rate limiting occurs at the HTTP boundary before password hashing.
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var id uuid.UUID
	if err = tx.QueryRow(ctx, "SELECT user_id FROM password_resets WHERE token_hash=$1 AND expires_at>now()", tokenHash(token)).Scan(&id); errors.Is(err, pgx.ErrNoRows) {
		return ErrResetToken
	} else if err != nil {
		return err
	}
	// Lock users before consuming tokens, matching change/reset request lock order.
	var verified bool
	if err = tx.QueryRow(ctx, "SELECT verified_at IS NOT NULL FROM users WHERE id=$1 FOR UPDATE", id).Scan(&verified); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, "DELETE FROM password_resets WHERE token_hash=$1 AND user_id=$2 AND expires_at>now()", tokenHash(token), id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 || !verified {
		return ErrResetToken
	}
	return s.replacePassword(ctx, tx, id, hash, "auth.password_reset", rid)
}

func (s *Service) ChangePassword(ctx context.Context, id uuid.UUID, session, current, password, rid string) error {
	if len(session) != 43 {
		return ErrCredentials
	}
	if err := s.RateLimit(ctx, "password-change:"+id.String(), 5); err != nil {
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var old string
	if err = tx.QueryRow(ctx, "SELECT password_hash FROM users WHERE id=$1 FOR UPDATE", id).Scan(&old); err != nil {
		return err
	}
	var active bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM sessions WHERE token_hash=$1 AND user_id=$2 AND expires_at>now())", tokenHash(session), id).Scan(&active); err != nil {
		return err
	}
	if !active {
		return ErrCredentials
	}
	if !CheckPassword(old, current) {
		return ErrCurrentPassword
	}
	return s.replacePassword(ctx, tx, id, hash, "auth.password_changed", rid)
}

func (s *Service) replacePassword(ctx context.Context, tx pgx.Tx, id uuid.UUID, hash, action, rid string) error {
	if _, err := tx.Exec(ctx, "UPDATE users SET password_hash=$2,updated_at=now() WHERE id=$1", id, hash); err != nil {
		return err
	}
	// Console tickets cascade from sessions; active relays already recheck sessions.
	if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE user_id=$1", id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM password_resets WHERE user_id=$1", id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO audit_logs(id,actor_id,action,request_id) VALUES($1,$2,$3,$4)", uuid.New(), id, action, rid); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) resetCipher() (cipher.AEAD, error) {
	if len(s.RecoveryKey) != 32 {
		return nil, ErrRecoveryUnavailable
	}
	block, err := aes.NewCipher(s.RecoveryKey)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func (s *Service) encryptReset(token string, digest []byte) ([]byte, error) {
	gcm, err := s.resetCipher()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(token), digest), nil
}
func (s *Service) decryptReset(data, digest []byte) (string, error) {
	gcm, err := s.resetCipher()
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() || len(data) > 128 {
		return "", ErrResetToken
	}
	raw, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], digest)
	if err != nil {
		return "", ErrResetToken
	}
	if len(raw) != 43 {
		return "", ErrResetToken
	}
	return string(raw), nil
}

// DeliverRecoveryOnce provides at-least-once delivery across API processes and
// restarts. Row locks prevent concurrent workers from sending the same email.
func (s *Service) DeliverRecoveryOnce(ctx context.Context, log *slog.Logger) (bool, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(context.Background())
	var id uuid.UUID
	var encrypted, digest []byte
	var email, rid string
	var attempts int
	err = tx.QueryRow(ctx, `SELECT o.id,o.encrypted_token,o.token_hash,u.email,o.request_id,o.attempts FROM recovery_mail o JOIN password_resets p USING(token_hash) JOIN users u ON u.id=p.user_id WHERE o.available_at<=now() AND p.expires_at>now() AND o.attempts<5 ORDER BY o.available_at FOR UPDATE OF o SKIP LOCKED LIMIT 1`).Scan(&id, &encrypted, &digest, &email, &rid, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	token, deliveryErr := s.decryptReset(encrypted, digest)
	if deliveryErr == nil {
		deliveryErr = s.Mailer.SendPasswordReset(ctx, email, token)
	}
	if deliveryErr != nil {
		if log != nil {
			log.Warn("password reset email delivery failed", "request_id", rid, "attempt", attempts+1)
		}
		_, err = tx.Exec(ctx, "UPDATE recovery_mail SET attempts=attempts+1,available_at=now()+$2*interval '1 second' WHERE id=$1", id, 5*(1<<attempts))
	} else {
		_, err = tx.Exec(ctx, "DELETE FROM recovery_mail WHERE id=$1", id)
	}
	if err != nil {
		return true, err
	}
	return true, tx.Commit(ctx)
}
func (s *Service) RunRecoveryDelivery(ctx context.Context, log *slog.Logger) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for n := 0; n < 10; n++ {
				deliveryCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
				found, err := s.DeliverRecoveryOnce(deliveryCtx, log)
				cancel()
				if err != nil {
					log.Error("recovery mail processing failed", "category", "database")
				}
				if err != nil || !found {
					break
				}
			}
		}
	}
}
