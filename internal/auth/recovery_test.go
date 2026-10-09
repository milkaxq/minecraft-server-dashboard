package auth

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/testutil"
)

type recoveryMailer struct {
	token string
	fail  bool
	sent  int
}

func (m *recoveryMailer) SendVerification(context.Context, string, string) error { return nil }
func (m *recoveryMailer) SendPasswordReset(_ context.Context, _ string, token string) error {
	if m.fail {
		return errors.New("delivery failure")
	}
	m.token = token
	m.sent++
	return nil
}
func recoveryFixture(t *testing.T) (*Service, *recoveryMailer, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	db := testutil.Database(t)
	m := &recoveryMailer{}
	s, err := New(db, m)
	if err != nil {
		t.Fatal(err)
	}
	s.RecoveryKey = bytes.Repeat([]byte{7}, 32)
	hash, err := HashPassword("original-password")
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if _, err = db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at) VALUES($1,'player@example.com',$2,'Player',now())", id, hash); err != nil {
		t.Fatal(err)
	}
	return s, m, id
}
func issueReset(t *testing.T, s *Service, m *recoveryMailer) string {
	t.Helper()
	ctx := context.Background()
	if err := s.RequestPasswordReset(ctx, "player@example.com", "request"); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.DeliverRecoveryOnce(ctx, nil); !ok || err != nil {
		t.Fatal(ok, err)
	}
	return m.token
}
func TestRecoveryPrivateDeliveryAndUnknownAccounts(t *testing.T) {
	s, m, id := recoveryFixture(t)
	ctx := context.Background()
	for _, email := range []string{"unknown@example.com", "player@example.com"} {
		if err := s.RequestPasswordReset(ctx, email, "request"); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	var encrypted, hash []byte
	if err := s.DB.QueryRow(ctx, "SELECT count(*) FROM password_resets").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if err := s.DB.QueryRow(ctx, "SELECT encrypted_token,token_hash FROM recovery_mail").Scan(&encrypted, &hash); err != nil {
		t.Fatal(err)
	}
	if m.sent != 0 {
		t.Fatal("public request performed synchronous delivery")
	}
	if ok, err := s.DeliverRecoveryOnce(ctx, nil); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if bytes.Contains(encrypted, []byte(m.token)) || !bytes.Equal(hash, tokenHash(m.token)) {
		t.Fatal("token storage is not protected")
	}
	if _, err := s.decryptReset(encrypted, bytes.Repeat([]byte{0}, 32)); err == nil {
		t.Fatal("ciphertext accepted for another token")
	}
	if _, err := s.DB.Exec(ctx, "UPDATE users SET verified_at=NULL WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err := s.RequestPasswordReset(ctx, "player@example.com", "unverified"); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.DeliverRecoveryOnce(ctx, nil); ok || err != nil {
		t.Fatal("unverified account received reset", ok, err)
	}
}
func TestResetRevokesSessionsTicketsAndAllResetLinks(t *testing.T) {
	s, m, id := recoveryFixture(t)
	ctx := context.Background()
	login, err := s.Login(ctx, "player@example.com", "original-password", "login")
	if err != nil {
		t.Fatal(err)
	}
	first := issueReset(t, s, m)
	second := issueReset(t, s, m)
	// Console tickets rely on the same session FK, so check the cascade explicitly.
	sid := uuid.New()
	if _, err = s.DB.Exec(ctx, `INSERT INTO servers(id,owner_id,plan_id,name,minecraft_version,server_type,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit) VALUES($1,$2,'10000000-0000-4000-8000-000000000001','Test','1.21.1','PAPER',2048,10240,100,0,0)`, sid, id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(ctx, "INSERT INTO console_tickets(token_hash,server_id,user_id,session_hash,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')", tokenHash("ticket"), sid, id, tokenHash(login.Token)); err != nil {
		t.Fatal(err)
	}
	if err = s.ResetPassword(ctx, first, "replacement-password", "reset"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, login.Token); !errors.Is(err, ErrCredentials) {
		t.Fatal("old session survived", err)
	}
	for _, token := range []string{first, second} {
		if err = s.ResetPassword(ctx, token, "another-password", "replay"); !errors.Is(err, ErrResetToken) {
			t.Fatal("reset link survived", err)
		}
	}
	for _, table := range []string{"sessions", "console_tickets", "password_resets", "recovery_mail"} {
		var count int
		if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
	if _, err = s.Login(ctx, "player@example.com", "original-password", "old"); !errors.Is(err, ErrCredentials) {
		t.Fatal("old password accepted", err)
	}
	if _, err = s.Login(ctx, "player@example.com", "replacement-password", "new"); err != nil {
		t.Fatal(err)
	}
	var auditCount int
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE action='auth.password_reset'").Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatal(auditCount, err)
	}
}
func TestExpiredResetAndWeakPasswordsDoNotConsumeValidLinks(t *testing.T) {
	s, m, _ := recoveryFixture(t)
	ctx := context.Background()
	token := issueReset(t, s, m)
	if err := s.ResetPassword(ctx, token, "short", "weak"); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(ctx, "UPDATE password_resets SET expires_at=now()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetPassword(ctx, token, "replacement-password", "expired"); !errors.Is(err, ErrResetToken) {
		t.Fatal(err)
	}
	if err := s.ResetPassword(ctx, "bad", "replacement-password", "bad"); !errors.Is(err, ErrResetToken) {
		t.Fatal(err)
	}
	if _, err := s.Login(ctx, "player@example.com", "original-password", "unchanged"); err != nil {
		t.Fatal(err)
	}
}
func TestConcurrentResetIsSingleUse(t *testing.T) {
	s, m, _ := recoveryFixture(t)
	token := issueReset(t, s, m)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- s.ResetPassword(context.Background(), token, "replacement-password", "concurrent")
		}()
	}
	wg.Wait()
	close(results)
	success, rejected := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrResetToken) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatal(success, rejected)
	}
}
func TestPasswordChangeRequiresCurrentPasswordAndOwnActiveSession(t *testing.T) {
	s, m, id := recoveryFixture(t)
	ctx := context.Background()
	login, err := s.Login(ctx, "player@example.com", "original-password", "login")
	if err != nil {
		t.Fatal(err)
	}
	token := issueReset(t, s, m)
	if err = s.ChangePassword(ctx, id, login.Token, "wrong-password", "replacement-password", "wrong"); !errors.Is(err, ErrCurrentPassword) {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, login.Token); err != nil {
		t.Fatal("wrong password revoked session", err)
	}
	if err = s.ChangePassword(ctx, id, "invalid-session", "original-password", "replacement-password", "invalid"); !errors.Is(err, ErrCredentials) {
		t.Fatal(err)
	}
	if err = s.ChangePassword(ctx, id, login.Token, "original-password", "replacement-password", "change"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, login.Token); !errors.Is(err, ErrCredentials) {
		t.Fatal(err)
	}
	if err = s.ResetPassword(ctx, token, "another-password", "stale"); !errors.Is(err, ErrResetToken) {
		t.Fatal("reset survives password change", err)
	}
}
func TestRecoveryDeliveryRetriesAndSurvivesServiceRestart(t *testing.T) {
	s, m, _ := recoveryFixture(t)
	ctx := context.Background()
	m.fail = true
	if err := s.RequestPasswordReset(ctx, "player@example.com", "delivery"); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.DeliverRecoveryOnce(ctx, nil); !ok || err != nil {
		t.Fatal(ok, err)
	}
	var attempts int
	if err := s.DB.QueryRow(ctx, "SELECT attempts FROM recovery_mail").Scan(&attempts); err != nil || attempts != 1 {
		t.Fatal(attempts, err)
	}
	if _, err := s.DB.Exec(ctx, "UPDATE recovery_mail SET available_at=now()"); err != nil {
		t.Fatal(err)
	}
	m.fail = false
	restarted := &Service{DB: s.DB, Mailer: m, RecoveryKey: s.RecoveryKey}
	if ok, err := restarted.DeliverRecoveryOnce(ctx, nil); !ok || err != nil || m.sent != 1 {
		t.Fatal(ok, err, m.sent)
	}
	if ok, err := restarted.DeliverRecoveryOnce(ctx, nil); ok || err != nil {
		t.Fatal("delivered twice", ok, err)
	}
}
func TestRecoveryAccountRateLimitDoesNotRevealExistence(t *testing.T) {
	s, _, _ := recoveryFixture(t)
	ctx := context.Background()
	for range 8 {
		for _, email := range []string{"player@example.com", "unknown@example.com"} {
			if err := s.RequestPasswordReset(ctx, email, "rate"); err != nil {
				t.Fatal(err)
			}
		}
	}
	var count int
	if err := s.DB.QueryRow(ctx, "SELECT count(*) FROM password_resets").Scan(&count); err != nil || count != 5 {
		t.Fatal(count, err)
	}
}
