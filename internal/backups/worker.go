package backups

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

type Worker struct {
	DB      *pgxpool.Pool
	Panel   pterodactyl.ManagementClient
	Control *control.Service
	Log     *slog.Logger
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		if err := w.Tick(ctx, time.Now().UTC()); err != nil && ctx.Err() == nil {
			w.Log.Error("backup scheduler tick failed", "category", "database_or_panel")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
func (w *Worker) Tick(ctx context.Context, now time.Time) error {
	if err := w.enqueue(ctx, now); err != nil {
		return err
	}
	seen := []uuid.UUID{}
	for i := 0; i < 10; i++ {
		r, err := w.claim(ctx, now, seen)
		if noRows(err) {
			return nil
		}
		if err != nil {
			return err
		}
		seen = append(seen, r.ID)
		err = w.process(ctx, r)
		w.release(r)
		if err != nil && ctx.Err() == nil {
			w.Log.Warn("backup run deferred", "run_id", r.ID, "server_id", r.ServerID, "category", "database_or_panel")
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return nil
}
func (w *Worker) process(parent context.Context, r claimedRun) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	// Renew a claim while remote operations are in flight. Loss of ownership
	// cancels the request; durable intents still protect a possibly sent mutation.
	done := make(chan struct{})
	defer func() { cancel(); <-done }()
	go func() {
		defer close(done)
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := w.renew(ctx, r); err != nil {
					cancel()
					return
				}
			}
		}
	}()
	conn, err := w.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	var held bool
	if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", "server:"+r.ServerID.String()).Scan(&held); err != nil {
		return err
	}
	if !held {
		return nil
	}
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		var released bool
		if e := conn.QueryRow(clean, "SELECT pg_advisory_unlock(hashtextextended($1,0))", "server:"+r.ServerID.String()).Scan(&released); e != nil || !released {
			_ = conn.Conn().Close(clean)
		}
	}()
	if err = w.renew(ctx, r); err != nil {
		return err
	}
	var owner uuid.UUID
	var identifier string
	var enabled bool
	var revision int64
	var limit int
	err = conn.QueryRow(ctx, `SELECT s.owner_id,COALESCE(s.pterodactyl_identifier,''),s.backup_limit,sc.enabled,sc.revision FROM servers s JOIN backup_schedules sc ON sc.server_id=s.id WHERE s.id=$1`, r.ServerID).Scan(&owner, &identifier, &limit, &enabled, &revision)
	if err != nil {
		return err
	}
	if identifier != r.Identifier {
		return w.transition(ctx, conn, r, "NEEDS_INSPECTION", "server_identity_changed", nil)
	}
	if r.BackupUUID != nil {
		return w.monitor(ctx, conn, r)
	}
	var intent bool
	if err = conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM backup_intents WHERE run_id=$1 AND action='CREATE')", r.ID).Scan(&intent); err != nil {
		return err
	}
	if intent {
		return w.transition(ctx, conn, r, "NEEDS_INSPECTION", "create_unconfirmed", nil)
	}
	if !enabled || revision != r.Revision || owner != r.Owner {
		return w.transition(ctx, conn, r, "SKIPPED", "schedule_changed", nil)
	}
	if limit < 2 {
		return w.transition(ctx, conn, r, "SKIPPED", "insufficient_allowance", nil)
	}
	if _, err = w.Control.Access(ctx, conn, owner, r.ServerID, server.Backups); err != nil {
		if errors.Is(err, control.ErrState) || errors.Is(err, server.ErrNotFound) {
			return w.transition(ctx, conn, r, "SKIPPED", "server_unavailable", nil)
		}
		return err
	}
	readyCtx, c := context.WithTimeout(ctx, 30*time.Second)
	err = w.Panel.Ready(readyCtx, r.Identifier)
	c()
	if err != nil {
		return w.retry(ctx, conn, r, "panel_unavailable")
	}
	// Reconcile a previous DELETE before capacity calculation. A missing target
	// proves deletion; a still-present target is never blindly deleted again.
	if err = w.reconcileDeletes(ctx, conn, r); err != nil {
		return err
	}
	var blocked bool
	if err = conn.QueryRow(ctx, `SELECT
 EXISTS(SELECT 1 FROM management_operations WHERE server_id=$1 AND action IN ('backup.create','world.backup') AND status IN ('PENDING','UNKNOWN'))
 OR EXISTS(SELECT 1 FROM backup_runs WHERE server_id=$1 AND id<>$2 AND status IN ('CREATING','NEEDS_INSPECTION'))
 OR EXISTS(SELECT 1 FROM backup_intents i JOIN backup_runs br ON br.id=i.run_id WHERE br.server_id=$1 AND br.id<>$2 AND i.status<>'CONFIRMED')`, r.ServerID, r.ID).Scan(&blocked); err != nil {
		return err
	}
	if blocked {
		return w.transition(ctx, conn, r, "SKIPPED", "pending_inspection", nil)
	}
	items, err := w.list(ctx, r.Identifier)
	if err != nil {
		return w.retry(ctx, conn, r, "panel_unavailable")
	}
	if len(items) >= limit {
		owned, err := w.owned(ctx, conn, r)
		if err != nil {
			return err
		}
		candidate, ok := RotationCandidate(items, owned)
		if !ok {
			return w.transition(ctx, conn, r, "SKIPPED", "capacity_full", nil)
		}
		// Only one slot is required. If the allowance was reduced below the live
		// count, skip rather than automatically delete several historical backups.
		if len(items) > limit {
			return w.transition(ctx, conn, r, "SKIPPED", "capacity_full", nil)
		}
		if err = w.rotate(ctx, conn, r, candidate); err != nil {
			return err
		}
		items, err = w.list(ctx, r.Identifier)
		if err != nil {
			return w.transition(ctx, conn, r, "NEEDS_INSPECTION", "delete_unconfirmed", nil)
		}
		for _, b := range items {
			if b.UUID == candidate {
				return w.transition(ctx, conn, r, "NEEDS_INSPECTION", "delete_unconfirmed", nil)
			}
		}
		if len(items) >= limit {
			return w.transition(ctx, conn, r, "SKIPPED", "capacity_full", nil)
		}
	}
	return w.create(ctx, conn, r)
}
func (w *Worker) list(ctx context.Context, id string) ([]pterodactyl.Backup, error) {
	request, c := context.WithTimeout(ctx, 30*time.Second)
	defer c()
	return w.Panel.ListBackups(request, id)
}
func (w *Worker) transition(ctx context.Context, conn *pgxpool.Conn, r claimedRun, status, code string, completed *time.Time) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	tag, err := tx.Exec(ctx, `UPDATE backup_runs SET status=$3,error_code=$4,completed_at=$5,updated_at=now(),retry_at=NULL WHERE id=$1 AND lease_token=$2 AND lease_expires_at>now()`, r.ID, r.Token, status, code, completed)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errLease
	}
	if status == "COMPLETED" {
		if _, err = tx.Exec(ctx, `UPDATE backup_schedules SET last_success=CASE WHEN last_success IS NULL OR last_success<$2 THEN $2 ELSE last_success END,updated_at=now() WHERE server_id=$1`, r.ServerID, completed); err != nil {
			return err
		}
	}
	if err = audit.Record(ctx, tx, r.Owner, r.ServerID, "backup.automatic."+status, "backup:"+r.ID.String(), map[string]any{"automation": true, "run_id": r.ID, "outcome": status, "error_code": code}); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err == nil {
		w.Log.Info("automatic backup transition", "run_id", r.ID, "server_id", r.ServerID, "status", status, "error_code", code)
	}
	return err
}
func (w *Worker) retry(ctx context.Context, conn *pgxpool.Conn, r claimedRun, code string) error {
	if r.Attempts >= 2 {
		return w.transition(ctx, conn, r, "FAILED", code, nil)
	}
	delay := []time.Duration{30 * time.Second, 120 * time.Second}[r.Attempts]
	tag, err := conn.Exec(ctx, "UPDATE backup_runs SET attempts=attempts+1,error_code=$3,retry_at=now()+$4::interval WHERE id=$1 AND lease_token=$2 AND lease_expires_at>now()", r.ID, r.Token, code, delay.String())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errLease
	}
	return nil
}
