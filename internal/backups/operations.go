package backups

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/pterodactyl"
)

func (w *Worker) owned(ctx context.Context, conn *pgxpool.Conn, r claimedRun) (map[string]bool, error) {
	rows, err := conn.Query(ctx, "SELECT backup_uuid::text FROM backup_runs WHERE server_id=$1 AND panel_identifier=$2 AND backup_uuid IS NOT NULL", r.ServerID, r.Identifier)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
func (w *Worker) intent(ctx context.Context, conn *pgxpool.Conn, r claimedRun, action string, target *uuid.UUID) (uuid.UUID, error) {
	// Derived operation keys survive restart, unlike randomly generated retries.
	key := uuid.NewSHA1(r.ID, []byte(action+fmt.Sprint(target)))
	tx, err := conn.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(context.Background())
	var valid bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM backup_runs WHERE id=$1 AND lease_token=$2 AND lease_expires_at>now())", r.ID, r.Token).Scan(&valid); err != nil {
		return uuid.Nil, err
	}
	if !valid {
		return uuid.Nil, errLease
	}
	if _, err = tx.Exec(ctx, "INSERT INTO backup_intents(id,run_id,action,target_uuid,status) VALUES($1,$2,$3,$4,'INTENDED')", key, r.ID, action, target); err != nil {
		return uuid.Nil, err
	}
	if err = audit.Record(ctx, tx, r.Owner, r.ServerID, "backup.automatic.intent", "backup:"+r.ID.String(), map[string]any{"automation": true, "run_id": r.ID, "operation_id": key, "action": action, "backup_uuid": target}); err != nil {
		return uuid.Nil, err
	}
	return key, tx.Commit(ctx)
}
func (w *Worker) create(ctx context.Context, conn *pgxpool.Conn, r claimedRun) error {
	op, err := w.intent(ctx, conn, r, "CREATE", nil)
	if err != nil {
		return err
	}
	// The owner/schedule/lifecycle check is repeated immediately before sending,
	// with the server lock held; the durable intent precedes every remote request.
	if err = w.revalidate(ctx, conn, r); err != nil {
		return w.rejectIntent(ctx, conn, r, op, "schedule_changed")
	}
	request, c := context.WithTimeout(ctx, 30*time.Second)
	b, err := w.Panel.CreateAutomaticBackup(request, r.Identifier, "Daily backup "+r.ID.String(), func(callCtx context.Context) error { return w.revalidate(callCtx, conn, r) })
	c()
	if err != nil {
		if errors.Is(err, pterodactyl.ErrBackupPreflight) {
			return w.rejectIntent(ctx, conn, r, op, "schedule_changed")
		}
		if pterodactyl.Rejected(err) || errors.Is(err, pterodactyl.ErrConflict) {
			return w.rejectIntent(ctx, conn, r, op, "panel_rejected")
		}
		return w.unknown(ctx, conn, r, op, "create_unconfirmed")
	}
	id, err := uuid.Parse(b.UUID)
	if err != nil || id == uuid.Nil {
		return w.unknown(ctx, conn, r, op, "create_unconfirmed")
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	tag, err := tx.Exec(ctx, `UPDATE backup_runs SET backup_uuid=$3,status='CREATING',error_code='',updated_at=now() WHERE id=$1 AND lease_token=$2 AND lease_expires_at>now()`, r.ID, r.Token, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errLease
	}
	if _, err = tx.Exec(ctx, "UPDATE backup_intents SET status='CONFIRMED',result_uuid=$2,updated_at=now() WHERE id=$1", op, id); err != nil {
		return err
	}
	if err = audit.Record(ctx, tx, r.Owner, r.ServerID, "backup.automatic.CREATING", "backup:"+r.ID.String(), map[string]any{"automation": true, "run_id": r.ID, "backup_uuid": id}); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	r.BackupUUID = &id
	return w.monitor(ctx, conn, r)
}
func (w *Worker) revalidate(ctx context.Context, conn *pgxpool.Conn, r claimedRun) error {
	var valid bool
	err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM backup_schedules sc JOIN servers s ON s.id=sc.server_id JOIN backup_runs br ON br.server_id=s.id
 WHERE br.id=$1 AND br.lease_token=$2 AND br.lease_expires_at>now()
 AND sc.enabled AND sc.revision=$3 AND s.owner_id=$4 AND s.pterodactyl_identifier=$5 AND s.backup_limit>=2 AND s.backup_limit=$6)`, r.ID, r.Token, r.Revision, r.Owner, r.Identifier, r.Limit).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return control.ErrState
	}
	_, err = w.Control.Access(ctx, conn, r.Owner, r.ServerID, "backups")
	return err
}
func (w *Worker) monitor(ctx context.Context, conn *pgxpool.Conn, r claimedRun) error {
	items, err := w.list(ctx, r.Identifier)
	if err != nil {
		return nil
	} // Retain CREATING and poll again, never re-create.
	for _, b := range items {
		if b.UUID != r.BackupUUID.String() {
			continue
		}
		if b.CompletedAt == nil {
			return nil
		}
		at, err := time.Parse(time.RFC3339Nano, *b.CompletedAt)
		if err != nil {
			return w.transition(ctx, conn, r, "NEEDS_INSPECTION", "completion_unconfirmed", nil)
		}
		if b.Successful {
			return w.transition(ctx, conn, r, "COMPLETED", "", &at)
		}
		return w.transition(ctx, conn, r, "FAILED", "backup_failed", &at)
	}
	return w.transition(ctx, conn, r, "NEEDS_INSPECTION", "backup_missing", nil)
}
func (w *Worker) rotate(ctx context.Context, conn *pgxpool.Conn, r claimedRun, target string) error {
	id, err := uuid.Parse(target)
	if err != nil {
		return w.transition(ctx, conn, r, "NEEDS_INSPECTION", "backup_identity_invalid", nil)
	}
	op, err := w.intent(ctx, conn, r, "DELETE", &id)
	if err != nil {
		return err
	}
	if err = w.revalidate(ctx, conn, r); err != nil {
		return w.rejectIntent(ctx, conn, r, op, "schedule_changed")
	}
	// Re-list after recording intent so changed lock/completion status cannot
	// turn the earlier policy snapshot into deletion authority.
	items, err := w.list(ctx, r.Identifier)
	if err != nil {
		return w.rejectIntent(ctx, conn, r, op, "panel_unavailable")
	}
	owned, err := w.owned(ctx, conn, r)
	if err != nil {
		return err
	}
	candidate, ok := RotationCandidate(items, owned)
	if !ok || candidate != target {
		return w.rejectIntent(ctx, conn, r, op, "capacity_full")
	}
	request, c := context.WithTimeout(ctx, 30*time.Second)
	err = w.Panel.DeleteAutomaticBackup(request, r.Identifier, target, func(callCtx context.Context) error { return w.revalidate(callCtx, conn, r) })
	c()
	if errors.Is(err, pterodactyl.ErrBackupPreflight) {
		return w.rejectIntent(ctx, conn, r, op, "schedule_changed")
	}
	if err != nil && (pterodactyl.Rejected(err) || errors.Is(err, pterodactyl.ErrConflict)) {
		return w.rejectIntent(ctx, conn, r, op, "panel_rejected")
	}
	// A successful HTTP response alone is not proof that a slot is free.
	listed, e := w.list(ctx, r.Identifier)
	if e == nil {
		found := false
		for _, b := range listed {
			if b.UUID == target {
				found = true
			}
		}
		if !found {
			return w.confirmDelete(ctx, conn, r, op, id)
		}
	}
	return w.unknown(ctx, conn, r, op, "delete_unconfirmed")
}
func (w *Worker) confirmDelete(ctx context.Context, conn *pgxpool.Conn, r claimedRun, op, id uuid.UUID) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	tag, err := tx.Exec(ctx, `UPDATE backup_intents SET status='CONFIRMED',updated_at=now() WHERE id=$1 AND EXISTS(SELECT 1 FROM backup_runs WHERE id=$2 AND lease_token=$3 AND lease_expires_at>now())`, op, r.ID, r.Token)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errLease
	}
	if err = audit.Record(ctx, tx, r.Owner, r.ServerID, "backup.automatic.deleted", "backup:"+r.ID.String(), map[string]any{"automation": true, "run_id": r.ID, "backup_uuid": id}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (w *Worker) unknown(ctx context.Context, conn *pgxpool.Conn, r claimedRun, op uuid.UUID, code string) error {
	if err := w.finishIntent(ctx, conn, r, op, "UNKNOWN"); err != nil {
		return err
	}
	if err := w.transition(ctx, conn, r, "NEEDS_INSPECTION", code, nil); err != nil {
		return err
	}
	return control.ErrUnknown
}
func (w *Worker) rejectIntent(ctx context.Context, conn *pgxpool.Conn, r claimedRun, op uuid.UUID, code string) error {
	// CONFIRMED means the outcome is established, including a known rejection.
	if err := w.finishIntent(ctx, conn, r, op, "CONFIRMED"); err != nil {
		return err
	}
	if err := w.transition(ctx, conn, r, "SKIPPED", code, nil); err != nil {
		return err
	}
	return control.ErrState
}
func (w *Worker) reconcileDeletes(ctx context.Context, conn *pgxpool.Conn, r claimedRun) error {
	rows, err := conn.Query(ctx, "SELECT id,target_uuid FROM backup_intents WHERE run_id=$1 AND action='DELETE' AND status<>'CONFIRMED'", r.ID)
	if err != nil {
		return err
	}
	type pending struct{ id, target uuid.UUID }
	intents := []pending{}
	for rows.Next() {
		var p pending
		if err = rows.Scan(&p.id, &p.target); err != nil {
			rows.Close()
			return err
		}
		intents = append(intents, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(intents) == 0 {
		return nil
	}
	items, err := w.list(ctx, r.Identifier)
	if err != nil {
		return err
	}
	for _, p := range intents {
		for _, b := range items {
			if b.UUID == p.target.String() {
				if err = w.unknown(ctx, conn, r, p.id, "delete_unconfirmed"); err != nil {
					return err
				}
				return control.ErrUnknown
			}
		}
		if err = w.confirmDelete(ctx, conn, r, p.id, p.target); err != nil {
			return err
		}
	}
	return nil
}

// All intent outcomes are fenced, including requests whose response was lost.
func (w *Worker) finishIntent(ctx context.Context, conn *pgxpool.Conn, r claimedRun, op uuid.UUID, status string) error {
	tag, err := conn.Exec(ctx, `UPDATE backup_intents SET status=$4,updated_at=now() WHERE id=$1 AND run_id=$2
 AND EXISTS(SELECT 1 FROM backup_runs WHERE id=$2 AND lease_token=$3 AND lease_expires_at>now())`, op, r.ID, r.Token, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errLease
	}
	return nil
}
