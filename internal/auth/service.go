package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/user"
)

type Mailer interface {
	SendVerification(context.Context, string, string) error
	SendPasswordReset(context.Context, string, string) error
}
type Service struct {
	DB          *pgxpool.Pool
	Mailer      Mailer
	RecoveryKey []byte
	InviteOnly  bool
	dummyHash   string
}

func New(db *pgxpool.Pool, mailer Mailer) (*Service, error) {
	h, err := HashPassword("unused-dummy-password")
	return &Service{DB: db, Mailer: mailer, dummyHash: h}, err
}

type RegisterInput struct {
	Email           string `json:"email"`
	Password        string `json:"password"`
	Name            string `json:"name"`
	InvitationToken string `json:"invitation_token,omitempty"`
}

func normalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || len(email) > 254 || !strings.Contains(email, "@") {
		return "", ErrInvalid
	}
	return email, nil
}
func (s *Service) Register(ctx context.Context, input RegisterInput, requestID string) (user.User, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return user.User{}, err
	}
	name := strings.TrimSpace(input.Name)
	if len(name) == 0 || len(name) > 80 {
		return user.User{}, ErrInvalid
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return user.User{}, ErrInvalid
		}
	}
	hash, err := HashPassword(input.Password)
	if err != nil {
		return user.User{}, err
	}
	token, digest, err := newToken()
	if err != nil {
		return user.User{}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return user.User{}, err
	}
	defer tx.Rollback(context.Background())
	var invitationID uuid.UUID
	if s.InviteOnly || input.InvitationToken != "" {
		if len(input.InvitationToken) != 43 {
			return user.User{}, ErrInvitation
		}
		err = tx.QueryRow(ctx, "SELECT id FROM beta_invitations WHERE token_hash=$1 AND email=$2 AND consumed_at IS NULL AND revoked_at IS NULL AND expires_at>now() FOR UPDATE", tokenHash(input.InvitationToken), email).Scan(&invitationID)
		if errors.Is(err, pgx.ErrNoRows) {
			return user.User{}, ErrInvitation
		}
		if err != nil {
			return user.User{}, err
		}
	}
	u := user.User{ID: uuid.New(), Email: email, Name: name}
	err = tx.QueryRow(ctx, "INSERT INTO users(id,email,password_hash,name) VALUES($1,$2,$3,$4) RETURNING created_at,is_admin", u.ID, email, hash, name).Scan(&u.CreatedAt, &u.IsAdmin)
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return user.User{}, ErrDuplicate
	}
	if err != nil {
		return user.User{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO email_verifications(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '24 hours')", digest, u.ID); err != nil {
		return user.User{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_logs(id,actor_id,action,request_id) VALUES($1,$2,'auth.registered',$3)", uuid.New(), u.ID, requestID); err != nil {
		return user.User{}, err
	}
	if invitationID != uuid.Nil {
		if _, err = tx.Exec(ctx, "UPDATE beta_invitations SET consumed_at=now() WHERE id=$1", invitationID); err != nil {
			return user.User{}, err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO audit_logs(id,actor_id,action,request_id,metadata) VALUES($1,$2,'beta.invitation_used',$3,jsonb_build_object('invitation_id',$4::text))", uuid.New(), u.ID, requestID, invitationID.String()); err != nil {
			return user.User{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return user.User{}, err
	}
	// Account persists if delivery fails. Resend can recover without losing the account.
	if err = s.Mailer.SendVerification(ctx, email, token); err != nil {
		return user.User{}, err
	}
	return u, nil
}
func (s *Service) Resend(ctx context.Context, email string) error {
	email, err := normalizeEmail(email)
	if err != nil {
		return ErrInvalid
	}
	token, digest, err := newToken()
	if err != nil {
		return err
	}
	tag, err := s.DB.Exec(ctx, `INSERT INTO email_verifications(token_hash,user_id,expires_at)
 SELECT $2,id,now()+interval '24 hours' FROM users WHERE email=$1 AND verified_at IS NULL
 ON CONFLICT(user_id) DO UPDATE SET token_hash=EXCLUDED.token_hash,expires_at=EXCLUDED.expires_at`, email, digest)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	return s.Mailer.SendVerification(ctx, email, token)
}
func (s *Service) Verify(ctx context.Context, token, requestID string) error {
	if len(token) != 43 {
		return ErrInvalid
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var id uuid.UUID
	err = tx.QueryRow(ctx, "DELETE FROM email_verifications WHERE token_hash=$1 AND expires_at>now() RETURNING user_id", tokenHash(token)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalid
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE users SET verified_at=now(),updated_at=now() WHERE id=$1", id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_logs(id,actor_id,action,request_id) VALUES($1,$2,'auth.verified',$3)", uuid.New(), id, requestID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type LoginResult struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      user.User `json:"user"`
}

func (s *Service) Login(ctx context.Context, email, password, requestID string) (LoginResult, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return LoginResult{}, ErrCredentials
	}
	// Per-account limits complement the direct-peer HTTP limit. Store only a digest.
	if err := s.RateLimit(ctx, fmt.Sprintf("login:%x", tokenHash(email)), 10); err != nil {
		return LoginResult{}, err
	}
	var u user.User
	var hash string
	err = s.DB.QueryRow(ctx, "SELECT id,email,name,verified_at,created_at,password_hash,is_admin FROM users WHERE email=$1", email).Scan(&u.ID, &u.Email, &u.Name, &u.VerifiedAt, &u.CreatedAt, &hash, &u.IsAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		CheckPassword(s.dummyHash, password)
		return LoginResult{}, ErrCredentials
	}
	if err != nil {
		return LoginResult{}, err
	}
	if !CheckPassword(hash, password) {
		return LoginResult{}, ErrCredentials
	}
	if u.VerifiedAt == nil {
		return LoginResult{}, ErrUnverified
	}
	token, digest, err := newToken()
	if err != nil {
		return LoginResult{}, err
	}
	expires := time.Now().UTC().Add(24 * time.Hour)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return LoginResult{}, err
	}
	defer tx.Rollback(context.Background())
	// Serialize session creation with password changes. A password checked before
	// a reset must not create a new session after that reset commits.
	var currentHash string
	var verified bool
	if err = tx.QueryRow(ctx, "SELECT password_hash,verified_at IS NOT NULL FROM users WHERE id=$1 FOR SHARE", u.ID).Scan(&currentHash, &verified); err != nil {
		return LoginResult{}, err
	}
	if currentHash != hash || !verified {
		return LoginResult{}, ErrCredentials
	}
	if _, err = tx.Exec(ctx, "DELETE FROM sessions WHERE user_id=$1 AND expires_at<=now()", u.ID); err != nil {
		return LoginResult{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,$3)", digest, u.ID, expires); err != nil {
		return LoginResult{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_logs(id,actor_id,action,request_id) VALUES($1,$2,'auth.login',$3)", uuid.New(), u.ID, requestID); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: token, ExpiresAt: expires, User: u}, tx.Commit(ctx)
}
func (s *Service) Authenticate(ctx context.Context, token string) (user.User, error) {
	if len(token) != 43 {
		return user.User{}, ErrCredentials
	}
	var u user.User
	err := s.DB.QueryRow(ctx, `SELECT u.id,u.email,u.name,u.verified_at,u.created_at,u.is_admin FROM users u JOIN sessions s ON s.user_id=u.id WHERE s.token_hash=$1 AND s.expires_at>now() AND u.verified_at IS NOT NULL`, tokenHash(token)).Scan(&u.ID, &u.Email, &u.Name, &u.VerifiedAt, &u.CreatedAt, &u.IsAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.User{}, ErrCredentials
	}
	return u, err
}
func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.DB.Exec(ctx, "DELETE FROM sessions WHERE token_hash=$1", tokenHash(token))
	return err
}

// PostgreSQL gives atomic, shared rate limits across API instances. Never trust X-Forwarded-For.
func (s *Service) RateLimit(ctx context.Context, bucket string, limit int) error {
	var attempts int
	err := s.DB.QueryRow(ctx, `INSERT INTO auth_rate_limits(bucket,window_start,attempts) VALUES($1,now(),1)
 ON CONFLICT(bucket) DO UPDATE SET attempts=CASE WHEN auth_rate_limits.window_start<=now()-interval '15 minutes' THEN 1 ELSE auth_rate_limits.attempts+1 END,
 window_start=CASE WHEN auth_rate_limits.window_start<=now()-interval '15 minutes' THEN now() ELSE auth_rate_limits.window_start END RETURNING attempts`, bucket).Scan(&attempts)
	if err != nil {
		return err
	}
	if attempts > limit {
		return ErrRateLimited
	}
	return nil
}
func (s *Service) Cleanup(ctx context.Context) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM console_tickets WHERE expires_at<=now(); DELETE FROM sessions WHERE expires_at<=now(); DELETE FROM email_verifications WHERE expires_at<=now(); DELETE FROM password_resets WHERE expires_at<=now(); DELETE FROM auth_rate_limits WHERE window_start<now()-interval '1 day';`)
	return err
}
