// Package jobs keeps commands durable in PostgreSQL, atomically with server states.
package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/server"
)

var ErrForbidden = errors.New("administrator access required")
var ErrConflict = errors.New("operation conflicts with current state")
var ErrInvalid = errors.New("invalid job input")

type Action string

const (
	Provision Action = "PROVISION"
	Suspend   Action = "SUSPEND"
	Unsuspend Action = "UNSUSPEND"
	Terminate Action = "TERMINATE"
	VoicePort Action = "VOICE_PORT"
)

type Input struct {
	Reason       string `json:"reason"`
	Confirmation string `json:"confirmation,omitempty"`
	// Only an operator who has confirmed no Panel operation remains in flight may reset create intent.
	RecoveryConfirmation string `json:"recovery_confirmation,omitempty"`
}
type Job struct {
	ID           uuid.UUID `json:"id"`
	ServerID     uuid.UUID `json:"server_id"`
	ActorID      uuid.UUID `json:"actor_id"`
	Action       Action    `json:"action"`
	Status       string    `json:"status"`
	ErrorCode    *string   `json:"error_code"`
	Attempts     int       `json:"attempts"`
	MaxAttempts  int       `json:"-"`
	RetryInstall bool      `json:"-"`
	RequestID    string    `json:"request_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

const columns = "id,server_id,actor_id,action,status,attempts,max_attempts,retry_install,request_id,created_at,updated_at,last_error"

func Scan(row pgx.Row) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.ServerID, &j.ActorID, &j.Action, &j.Status, &j.Attempts, &j.MaxAttempts, &j.RetryInstall, &j.RequestID, &j.CreatedAt, &j.UpdatedAt, &j.ErrorCode)
	return j, err
}

type Service struct{ DB *pgxpool.Pool }

func (s Service) RequireAdmin(ctx context.Context, id uuid.UUID) error {
	var allowed bool
	err := s.DB.QueryRow(ctx, "SELECT is_admin FROM users WHERE id=$1 AND verified_at IS NOT NULL", id).Scan(&allowed)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}
func (s Service) Enqueue(ctx context.Context, actor, id, key uuid.UUID, action Action, input Input, requestID string) (Job, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(context.Background())
	job, err := s.EnqueueTx(ctx, tx, actor, id, key, action, input, requestID)
	if err != nil {
		return Job{}, err
	}
	return job, tx.Commit(ctx)
}

// EnqueueTx lets private-beta creation commit its server and job together.
func (s Service) EnqueueTx(ctx context.Context, tx pgx.Tx, actor, id, key uuid.UUID, action Action, input Input, requestID string) (Job, error) {
	if key == uuid.Nil || actor == uuid.Nil || id == uuid.Nil {
		return Job{}, ErrInvalid
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if len(input.Reason) < 3 || len(input.Reason) > 500 {
		return Job{}, ErrInvalid
	}
	if action != Provision && action != Suspend && action != Unsuspend && action != Terminate && action != VoicePort {
		return Job{}, ErrInvalid
	}
	if action == Terminate && input.Confirmation != id.String() {
		return Job{}, ErrInvalid
	}
	if input.RecoveryConfirmation != "" && ((action != Provision && action != Terminate) || input.RecoveryConfirmation != "PANEL_OPERATIONS_FINISHED_AND_ABSENT") {
		return Job{}, ErrInvalid
	}
	var admin bool
	err := tx.QueryRow(ctx, "SELECT is_admin FROM users WHERE id=$1 AND verified_at IS NOT NULL", actor).Scan(&admin)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrForbidden
	}
	if err != nil {
		return Job{}, err
	}
	var state server.State
	err = tx.QueryRow(ctx, "SELECT status FROM servers WHERE id=$1 FOR UPDATE", id).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, server.ErrNotFound
	}
	if err != nil {
		return Job{}, err
	}
	if action == VoicePort {
		var role server.Role
		var kind string
		err = tx.QueryRow(ctx, `SELECT CASE WHEN s.owner_id=$1 THEN 'OWNER' ELSE COALESCE(m.role,'') END,s.server_type FROM servers s LEFT JOIN server_members m ON m.server_id=s.id AND m.user_id=$1 WHERE s.id=$2`, actor, id).Scan(&role, &kind)
		if err != nil {
			return Job{}, err
		}
		if !server.Allowed(role, server.Settings) {
			return Job{}, server.ErrNotFound
		}
		if !server.SupportsPlugins(kind) && !server.SupportsMods(kind) {
			return Job{}, ErrInvalid
		}
	} else if !admin {
		if action != Terminate {
			return Job{}, ErrForbidden
		}
		var owner uuid.UUID
		if err = tx.QueryRow(ctx, "SELECT owner_id FROM servers WHERE id=$1", id).Scan(&owner); err != nil {
			return Job{}, err
		}
		if owner != actor {
			return Job{}, server.ErrNotFound
		}
	}
	body, _ := json.Marshal(struct {
		Action Action
		Input  Input
	}{action, input})
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(body))
	var existingFingerprint string
	err = tx.QueryRow(ctx, "SELECT request_fingerprint FROM jobs WHERE server_id=$1 AND idempotency_key=$2", id, key).Scan(&existingFingerprint)
	if err == nil {
		if existingFingerprint != fingerprint {
			return Job{}, ErrConflict
		}
		job, err := Scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM jobs WHERE server_id=$1 AND idempotency_key=$2", id, key))
		return job, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Job{}, err
	}
	var active bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM jobs WHERE server_id=$1 AND status IN ('PENDING','RUNNING'))", id).Scan(&active); err != nil {
		return Job{}, err
	}
	if active {
		return Job{}, ErrConflict
	}
	var entitled bool
	if err = tx.QueryRow(ctx, "SELECT NOT EXISTS(SELECT 1 FROM subscriptions WHERE server_id=$1 AND (status<>'ACTIVE' OR current_period_end<=now()))", id).Scan(&entitled); err != nil {
		return Job{}, err
	}
	if (action == Unsuspend || action == Provision || action == VoicePort) && !entitled {
		return Job{}, ErrConflict
	}
	if action == Provision {
		var unpaid bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM orders WHERE server_id=$1 AND kind='NEW' AND status<>'PAID')", id).Scan(&unpaid); err != nil {
			return Job{}, err
		}
		if unpaid {
			return Job{}, ErrConflict
		}
	}
	next := state
	switch action {
	case VoicePort:
		if state != server.Running {
			return Job{}, ErrConflict
		}
	case Provision:
		if state == server.PendingPayment {
			next = server.PaymentConfirmed
		} else if state == server.Failed {
			next = server.Provisioning
		} else {
			return Job{}, ErrConflict
		}
	case Suspend:
		if state != server.Running {
			return Job{}, ErrConflict
		}
	case Unsuspend:
		if state != server.Suspended {
			return Job{}, ErrConflict
		}
	case Terminate:
		if state == server.Cancelled || state == server.Provisioning {
			return Job{}, ErrConflict
		}
	}
	if input.RecoveryConfirmation != "" {
		if state != server.Failed {
			return Job{}, ErrConflict
		}
		var owner uuid.UUID
		if err = tx.QueryRow(ctx, "SELECT owner_id FROM servers WHERE id=$1", id).Scan(&owner); err != nil {
			return Job{}, err
		}
		// Use the same user advisory key as the worker before resetting a shared user's intent.
		if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "user:"+owner.String()); err != nil {
			return Job{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE users SET panel_create_attempted=false WHERE id=$1 AND pterodactyl_user_id IS NULL", owner); err != nil {
			return Job{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE servers SET panel_create_attempted=false WHERE id=$1 AND pterodactyl_server_id IS NULL", id); err != nil {
			return Job{}, err
		}
	}
	if next != state {
		if !server.CanTransition(state, next) {
			return Job{}, ErrConflict
		}
		if _, err = tx.Exec(ctx, "UPDATE servers SET status=$2,internal_error=NULL,updated_at=now() WHERE id=$1", id, next); err != nil {
			return Job{}, err
		}
	}
	if action == Suspend {
		if _, err = tx.Exec(ctx, "UPDATE servers SET suspension_reason=CASE WHEN NOT $2 AND suspension_reason='BILLING' THEN 'BILLING' ELSE 'ADMIN' END WHERE id=$1", id, entitled); err != nil {
			return Job{}, err
		}
	}
	if action == Terminate {
		if _, err = tx.Exec(ctx, "UPDATE subscriptions SET status='CANCELLED',updated_at=now() WHERE server_id=$1", id); err != nil {
			return Job{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE orders SET status='CANCELLED',updated_at=now() WHERE server_id=$1 AND status='PENDING'", id); err != nil {
			return Job{}, err
		}
	}
	job, err := Scan(tx.QueryRow(ctx, `INSERT INTO jobs(id,server_id,actor_id,action,idempotency_key,request_fingerprint,request_id,retry_install) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+columns, uuid.New(), id, actor, action, key, fingerprint, requestID, state == server.Failed))
	if err != nil {
		return Job{}, err
	}
	metadata := map[string]any{"reason": input.Reason, "job_id": job.ID, "from": state, "to": next, "recovery_confirmation": input.RecoveryConfirmation}
	if err = audit.Record(ctx, tx, actor, id, "job.enqueued."+strings.ToLower(string(action)), requestID, metadata); err != nil {
		return Job{}, err
	}
	return job, nil
}
func (s Service) Get(ctx context.Context, actor, id uuid.UUID) (Job, error) {
	if err := s.RequireAdmin(ctx, actor); err != nil {
		return Job{}, err
	}
	j, err := Scan(s.DB.QueryRow(ctx, "SELECT "+columns+" FROM jobs WHERE id=$1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, server.ErrNotFound
	}
	return j, err
}
