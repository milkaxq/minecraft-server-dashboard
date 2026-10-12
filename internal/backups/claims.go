package backups

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type claimedRun struct {
	ID, ServerID, Owner, Token uuid.UUID
	Revision                   int64
	Identifier, Status         string
	BackupUUID                 *uuid.UUID
	Attempts                   int
}

// enqueue commits at most one missed daily instant per schedule and advances to
// the next future day in the same transaction, independently of provisioning.
func (w *Worker) enqueue(ctx context.Context, now time.Time) error {
	tx, err := w.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	rows, err := tx.Query(ctx, `SELECT sc.server_id,sc.revision,sc.next_run,sc.hour,sc.minute,sc.timezone,s.owner_id,COALESCE(s.pterodactyl_identifier,'')
 FROM backup_schedules sc JOIN servers s ON s.id=sc.server_id
 WHERE sc.enabled AND sc.next_run<=$1 ORDER BY sc.next_run LIMIT 10 FOR UPDATE OF sc SKIP LOCKED`, now)
	if err != nil {
		return err
	}
	type dueSchedule struct {
		id, owner        uuid.UUID
		revision         int64
		at               time.Time
		hour, minute     int
		zone, identifier string
	}
	due := []dueSchedule{}
	for rows.Next() {
		var d dueSchedule
		if err = rows.Scan(&d.id, &d.revision, &d.at, &d.hour, &d.minute, &d.zone, &d.owner, &d.identifier); err != nil {
			rows.Close()
			return err
		}
		due = append(due, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, d := range due {
		next, e := NextDaily(now, d.hour, d.minute, d.zone)
		if e != nil {
			return e
		}
		if _, err = tx.Exec(ctx, `INSERT INTO backup_runs(id,server_id,schedule_revision,scheduled_at,owner_id,panel_identifier,status)
 VALUES($1,$2,$3,$4,$5,$6,'QUEUED') ON CONFLICT(server_id,scheduled_at) DO NOTHING`, uuid.New(), d.id, d.revision, d.at, d.owner, d.identifier); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE backup_schedules SET next_run=$2,updated_at=now() WHERE server_id=$1", d.id, next); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (w *Worker) claim(ctx context.Context, now time.Time, seen []uuid.UUID) (claimedRun, error) {
	var r claimedRun
	tx, err := w.DB.Begin(ctx)
	if err != nil {
		return r, err
	}
	defer tx.Rollback(context.Background())
	err = tx.QueryRow(ctx, `SELECT id,server_id,owner_id,schedule_revision,panel_identifier,status,backup_uuid,attempts
 FROM backup_runs WHERE status IN ('QUEUED','CREATING')
 AND (lease_expires_at IS NULL OR lease_expires_at<$1)
 AND (retry_at IS NULL OR retry_at<=$1) AND NOT(id=ANY($2::uuid[]))
 ORDER BY scheduled_at LIMIT 1 FOR UPDATE SKIP LOCKED`, now, seen).Scan(&r.ID, &r.ServerID, &r.Owner, &r.Revision, &r.Identifier, &r.Status, &r.BackupUUID, &r.Attempts)
	if err != nil {
		return r, err
	}
	r.Token = uuid.New()
	if _, err = tx.Exec(ctx, "UPDATE backup_runs SET lease_token=$2,lease_expires_at=$3 WHERE id=$1", r.ID, r.Token, now.Add(90*time.Second)); err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}
func (w *Worker) release(r claimedRun) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := w.DB.Exec(ctx, "UPDATE backup_runs SET lease_token=NULL,lease_expires_at=NULL WHERE id=$1 AND lease_token=$2", r.ID, r.Token)
	if err != nil {
		w.Log.Warn("backup lease release failed", "run_id", r.ID, "category", "database")
	}
}

var errLease = errors.New("backup lease lost")

func (w *Worker) renew(ctx context.Context, r claimedRun) error {
	tag, err := w.DB.Exec(ctx, "UPDATE backup_runs SET lease_expires_at=now()+interval '90 seconds' WHERE id=$1 AND lease_token=$2 AND lease_expires_at>now()", r.ID, r.Token)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errLease
	}
	return nil
}
func noRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
