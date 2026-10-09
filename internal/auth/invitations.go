package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/audit"
)

var ErrInvitation = errors.New("valid invitation required")
var ErrInviteAdmin = errors.New("administrator access required")

type Invitation struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}
type IssuedInvitation struct {
	Invitation
	Token string `json:"token"`
}

func invitationAdmin(ctx context.Context, tx pgx.Tx, actor uuid.UUID) error {
	var allowed bool
	err := tx.QueryRow(ctx, "SELECT is_admin FROM users WHERE id=$1 AND verified_at IS NOT NULL FOR SHARE", actor).Scan(&allowed)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInviteAdmin
	}
	if err != nil {
		return err
	}
	if !allowed {
		return ErrInviteAdmin
	}
	return nil
}
func (s *Service) CreateInvitation(ctx context.Context, actor uuid.UUID, email, rid string) (IssuedInvitation, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return IssuedInvitation{}, err
	}
	defer tx.Rollback(context.Background())
	if err = invitationAdmin(ctx, tx, actor); err != nil {
		return IssuedInvitation{}, err
	}
	email, err = normalizeEmail(email)
	if err != nil {
		return IssuedInvitation{}, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)", email).Scan(&exists); err != nil {
		return IssuedInvitation{}, err
	}
	if exists {
		return IssuedInvitation{}, ErrDuplicate
	}
	token, digest, err := newToken()
	if err != nil {
		return IssuedInvitation{}, err
	}
	result := IssuedInvitation{Invitation: Invitation{ID: uuid.New(), Email: email, Status: "ACTIVE"}, Token: token}
	err = tx.QueryRow(ctx, "INSERT INTO beta_invitations(id,email,token_hash,created_by,expires_at) VALUES($1,$2,$3,$4,now()+interval '7 days') RETURNING created_at,expires_at", result.ID, email, digest, actor).Scan(&result.CreatedAt, &result.ExpiresAt)
	if err != nil {
		return IssuedInvitation{}, err
	}
	if err = audit.Record(ctx, tx, actor, uuid.Nil, "beta.invitation_created", rid, map[string]string{"invitation_id": result.ID.String()}); err != nil {
		return IssuedInvitation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return IssuedInvitation{}, err
	}
	return result, nil
}
func (s *Service) Invitations(ctx context.Context, actor uuid.UUID) ([]Invitation, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	if err = invitationAdmin(ctx, tx, actor); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id,email,created_at,expires_at,CASE WHEN consumed_at IS NOT NULL THEN 'USED' WHEN revoked_at IS NOT NULL THEN 'REVOKED' WHEN expires_at<=now() THEN 'EXPIRED' ELSE 'ACTIVE' END FROM beta_invitations ORDER BY created_at DESC,id LIMIT 100`)
	if err != nil {
		return nil, err
	}
	out := []Invitation{}
	for rows.Next() {
		var item Invitation
		if err = rows.Scan(&item.ID, &item.Email, &item.CreatedAt, &item.ExpiresAt, &item.Status); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *Service) RevokeInvitation(ctx context.Context, actor, id uuid.UUID, rid string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err = invitationAdmin(ctx, tx, actor); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, "UPDATE beta_invitations SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrInvitation
	}
	if err = audit.Record(ctx, tx, actor, uuid.Nil, "beta.invitation_revoked", rid, map[string]string{"invitation_id": id.String()}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// BootstrapInvitation is only called by the privileged operator CLI. It never
// opens public registration and is disabled as soon as any account exists.
func BootstrapInvitation(ctx context.Context, db *pgxpool.Pool, email, rid string) (IssuedInvitation, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return IssuedInvitation{}, err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return IssuedInvitation{}, err
	}
	defer tx.Rollback(context.Background())
	// Serialize first-account bootstrap with other bootstraps and registration.
	if _, err = tx.Exec(ctx, "LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		return IssuedInvitation{}, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users) OR EXISTS(SELECT 1 FROM beta_invitations WHERE created_by IS NULL AND consumed_at IS NULL AND revoked_at IS NULL AND expires_at>now())").Scan(&exists); err != nil {
		return IssuedInvitation{}, err
	}
	if exists {
		return IssuedInvitation{}, ErrInviteAdmin
	}
	token, digest, err := newToken()
	if err != nil {
		return IssuedInvitation{}, err
	}
	out := IssuedInvitation{Invitation: Invitation{ID: uuid.New(), Email: email, Status: "ACTIVE"}, Token: token}
	if err = tx.QueryRow(ctx, "INSERT INTO beta_invitations(id,email,token_hash,expires_at) VALUES($1,$2,$3,now()+interval '7 days') RETURNING created_at,expires_at", out.ID, email, digest).Scan(&out.CreatedAt, &out.ExpiresAt); err != nil {
		return IssuedInvitation{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_logs(id,action,request_id,metadata) VALUES($1,'operator.bootstrap_invitation',$2,jsonb_build_object('invitation_id',$3::text,'source','privileged_cli'))", uuid.New(), rid, out.ID.String()); err != nil {
		return IssuedInvitation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return IssuedInvitation{}, err
	}
	return out, nil
}
