package billing

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/server"
)

// Reconcile is safe to run concurrently across API replicas. Access is denied at
// the exact entitlement deadline; durable Panel suspension is eventual via worker.
func (s *Service) Reconcile(ctx context.Context) (int, error) {
	rows, err := s.DB.Query(ctx, `SELECT s.id FROM servers s JOIN subscriptions su ON su.server_id=s.id WHERE s.status<>'CANCELLED' AND ((su.current_period_end<=now() AND (su.status='ACTIVE' OR (s.status='RUNNING' AND NOT EXISTS(SELECT 1 FROM jobs j WHERE j.server_id=s.id AND (j.status IN ('PENDING','RUNNING') OR (j.status='FAILED' AND j.action='SUSPEND' AND j.request_id='billing-reconcile' AND j.created_at>=su.current_period_end)))))) OR (s.status='SUSPENDED' AND s.suspension_reason='BILLING' AND su.status='ACTIVE' AND su.current_period_end>now() AND NOT EXISTS(SELECT 1 FROM jobs j WHERE j.server_id=s.id AND (j.status IN ('PENDING','RUNNING') OR (j.status='FAILED' AND j.action='UNSUSPEND' AND j.created_at>=su.updated_at))))) ORDER BY su.current_period_end,s.id LIMIT 100`)
	if err != nil {
		return 0, err
	}
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	count := 0
	for _, id := range ids {
		if err = ctx.Err(); err != nil {
			return count, err
		}
		if err = s.reconcileOne(ctx, id); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
func (s *Service) reconcileOne(ctx context.Context, id uuid.UUID) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var user uuid.UUID
	if err = tx.QueryRow(ctx, "SELECT owner_id FROM servers WHERE id=$1 FOR UPDATE", id).Scan(&user); err != nil {
		return err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, "SELECT now()").Scan(&now); err != nil {
		return err
	}
	if err = reconcileServer(ctx, tx, user, id, now, "billing-reconcile"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func reconcileServer(ctx context.Context, tx pgx.Tx, user, id uuid.UUID, now time.Time, rid string) error {
	var state server.State
	var reason *string
	if err := tx.QueryRow(ctx, "SELECT status,suspension_reason FROM servers WHERE id=$1", id).Scan(&state, &reason); err != nil {
		return err
	}
	if state == server.Cancelled {
		return nil
	}
	var subStatus string
	var end time.Time
	var cancel bool
	err := tx.QueryRow(ctx, "SELECT status,current_period_end,cancel_at_period_end FROM subscriptions WHERE server_id=$1 FOR UPDATE", id).Scan(&subStatus, &end, &cancel)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	entitled := subStatus == "ACTIVE" && end.After(now)
	if !end.After(now) && subStatus == "ACTIVE" {
		next := "PAST_DUE"
		if cancel {
			next = "CANCELLED"
		}
		if _, err = tx.Exec(ctx, "UPDATE subscriptions SET status=$2,updated_at=now() WHERE server_id=$1", id, next); err != nil {
			return err
		}
		if err = audit.Record(ctx, tx, user, id, "subscription.expired", rid, map[string]any{"status": next, "period_end": end}); err != nil {
			return err
		}
	}
	var active bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM jobs WHERE server_id=$1 AND status IN ('PENDING','RUNNING'))", id).Scan(&active); err != nil {
		return err
	}
	if active {
		return nil
	}
	if !entitled && state == server.Running {
		if _, err = tx.Exec(ctx, "UPDATE servers SET suspension_reason='BILLING',updated_at=now() WHERE id=$1", id); err != nil {
			return err
		}
		// Automatic failures remain visible for admin retry; never flood the queue with
		// repeated failed commands for the same expired period.
		return queue(ctx, tx, user, id, jobs.Suspend, "expiry:"+id.String()+":"+end.UTC().Format(time.RFC3339Nano), rid)
	}
	if entitled && state == server.Suspended && reason != nil && *reason == "BILLING" {
		return queue(ctx, tx, user, id, jobs.Unsuspend, "renewal:"+id.String()+":"+end.UTC().Format(time.RFC3339Nano), rid)
	}
	return nil
}
