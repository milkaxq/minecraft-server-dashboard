package auth

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/testutil"
)

func betaFixture(t *testing.T) (*Service, uuid.UUID) {
	t.Helper()
	s, _, actor := recoveryFixture(t)
	s.InviteOnly = true
	if _, err := s.DB.Exec(context.Background(), "UPDATE users SET is_admin=true WHERE id=$1", actor); err != nil {
		t.Fatal(err)
	}
	return s, actor
}
func betaInput(email, token string) RegisterInput {
	return RegisterInput{Email: email, Name: "Beta player", Password: "a-long-beta-password", InvitationToken: token}
}
func TestInvitationGateEmailBindingExpiryRevocationAndSecrets(t *testing.T) {
	s, actor := betaFixture(t)
	ctx := context.Background()
	if _, err := s.Register(ctx, betaInput("guest@example.com", ""), "missing"); !errors.Is(err, ErrInvitation) {
		t.Fatal(err)
	}
	invite, err := s.CreateInvitation(ctx, actor, " Guest@Example.com ", "create")
	if err != nil {
		t.Fatal(err)
	}
	if invite.Email != "guest@example.com" || len(invite.Token) != 43 {
		t.Fatal("bad invitation")
	}
	if _, err = s.Register(ctx, betaInput("outsider@example.com", invite.Token), "wrong-email"); !errors.Is(err, ErrInvitation) {
		t.Fatal(err)
	}
	var digest []byte
	if err = s.DB.QueryRow(ctx, "SELECT token_hash FROM beta_invitations WHERE id=$1", invite.ID).Scan(&digest); err != nil || len(digest) != 32 {
		t.Fatal("invalid stored token", err)
	}
	if _, err = s.DB.Exec(ctx, "UPDATE beta_invitations SET expires_at=now()-interval '1 second' WHERE id=$1", invite.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Register(ctx, betaInput(invite.Email, invite.Token), "expired"); !errors.Is(err, ErrInvitation) {
		t.Fatal(err)
	}
	other, err := s.CreateInvitation(ctx, actor, "other@example.com", "other")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RevokeInvitation(ctx, actor, other.ID, "revoke"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Register(ctx, betaInput(other.Email, other.Token), "revoked"); !errors.Is(err, ErrInvitation) {
		t.Fatal(err)
	}
	list, err := s.Invitations(ctx, actor)
	if err != nil || len(list) != 2 {
		t.Fatal(err)
	}
	if list[0].Status != "REVOKED" || list[1].Status != "EXPIRED" {
		t.Fatal("invalid statuses", list)
	}
	var count int
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil || count != 1 {
		t.Fatal("gate created users", count, err)
	}
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE metadata::text LIKE '%'||$1||'%'", invite.Token).Scan(&count); err != nil || count != 0 {
		t.Fatal("token in audit", err)
	}
}
func TestInvitationConsumptionIsAtomicAndRequiresVerification(t *testing.T) {
	s, actor := betaFixture(t)
	ctx := context.Background()
	inv, err := s.CreateInvitation(ctx, actor, "guest@example.com", "create")
	if err != nil {
		t.Fatal(err)
	}
	weak := betaInput(inv.Email, inv.Token)
	weak.Password = "short"
	if _, err = s.Register(ctx, weak, "weak"); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.Register(ctx, betaInput(inv.Email, inv.Token), "register")
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	success, denied := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, ErrInvitation) {
			denied++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || denied != 1 {
		t.Fatal(success, denied)
	}
	if _, err = s.Login(ctx, inv.Email, "a-long-beta-password", "login"); !errors.Is(err, ErrUnverified) {
		t.Fatal("invitation bypassed email verification", err)
	}
	var admin bool
	var servers int
	if err = s.DB.QueryRow(ctx, "SELECT is_admin FROM users WHERE email=$1", inv.Email).Scan(&admin); err != nil || admin {
		t.Fatal("privileges granted", err)
	}
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM servers").Scan(&servers); err != nil || servers != 0 {
		t.Fatal("server access automatically granted", err)
	}
	list, err := s.Invitations(ctx, actor)
	if err != nil || list[0].Status != "USED" {
		t.Fatal(err)
	}
}
func TestInvitationAdminAuthorizationAndRevocation(t *testing.T) {
	s, actor := betaFixture(t)
	ctx := context.Background()
	inv, err := s.CreateInvitation(ctx, actor, "guest@example.com", "create")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(ctx, "UPDATE users SET is_admin=false WHERE id=$1", actor); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateInvitation(ctx, actor, "other@example.com", "create"); !errors.Is(err, ErrInviteAdmin) {
		t.Fatal(err)
	}
	if _, err = s.Invitations(ctx, actor); !errors.Is(err, ErrInviteAdmin) {
		t.Fatal(err)
	}
	if err = s.RevokeInvitation(ctx, actor, inv.ID, "revoke"); !errors.Is(err, ErrInviteAdmin) {
		t.Fatal(err)
	}
}
func TestDuplicateAccountRollsBackInvitationConsumption(t *testing.T) {
	s, actor := betaFixture(t)
	ctx := context.Background()
	inv, err := s.CreateInvitation(ctx, actor, "guest@example.com", "create")
	if err != nil {
		t.Fatal(err)
	}
	s.InviteOnly = false
	if _, err = s.Register(ctx, betaInput(inv.Email, ""), "open"); err != nil {
		t.Fatal(err)
	}
	s.InviteOnly = true
	if _, err = s.Register(ctx, betaInput(inv.Email, inv.Token), "duplicate"); !errors.Is(err, ErrDuplicate) {
		t.Fatal(err)
	}
	list, err := s.Invitations(ctx, actor)
	if err != nil || list[0].Status != "ACTIVE" {
		t.Fatal("failed transaction consumed invitation", err)
	}
}

func TestBootstrapInvitationIsFreshDatabaseOnlyAndGrantsNoAdmin(t *testing.T) {
	ctx := context.Background()
	db := testutil.Database(t)
	invite, err := BootstrapInvitation(ctx, db, "first@example.com", "bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = BootstrapInvitation(ctx, db, "second@example.com", "again"); !errors.Is(err, ErrInviteAdmin) {
		t.Fatal("second bootstrap allowed", err)
	}
	s, err := New(db, &recoveryMailer{})
	if err != nil {
		t.Fatal(err)
	}
	s.InviteOnly = true
	u, err := s.Register(ctx, betaInput(invite.Email, invite.Token), "register")
	if err != nil {
		t.Fatal(err)
	}
	if u.IsAdmin || u.VerifiedAt != nil {
		t.Fatal("bootstrap bypassed promotion or verification")
	}
	if _, err = BootstrapInvitation(ctx, db, "third@example.com", "after-user"); !errors.Is(err, ErrInviteAdmin) {
		t.Fatal("bootstrap with users allowed", err)
	}
}
