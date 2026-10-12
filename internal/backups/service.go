package backups

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/server"
)

var ErrRevision = errors.New("backup schedule changed")
var ErrAllowance = errors.New("automatic backups require at least two slots")

type Update struct {
	Enabled  bool   `json:"enabled"`
	Hour     int    `json:"hour"`
	Minute   int    `json:"minute"`
	Timezone string `json:"timezone"`
	Revision int64  `json:"revision"`
}
type Run struct {
	ID          uuid.UUID  `json:"id"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	Status      string     `json:"status"`
	BackupUUID  *uuid.UUID `json:"backup_uuid,omitempty"`
	ErrorCode   string     `json:"error_code"`
	CompletedAt *time.Time `json:"completed_at"`
}
type View struct {
	Update
	CanManage   bool       `json:"can_manage"`
	NextRun     *time.Time `json:"next_run"`
	LastSuccess *time.Time `json:"last_success"`
	Runs        []Run      `json:"runs"`
	Warning     string     `json:"warning"`
}
type Service struct {
	DB      *pgxpool.Pool
	Control *control.Service
}

// Status access deliberately does not require a live Panel: owners must be
// able to disable schedules and inspect failures on suspended servers.
func (s *Service) Get(ctx context.Context, actor, id uuid.UUID) (View, error) {
	v := View{Update: Update{Hour: 3, Timezone: "UTC"}, Runs: []Run{}}
	var role server.Role
	var state string
	var limit int
	var eligible bool
	err := s.DB.QueryRow(ctx, `SELECT CASE WHEN s.owner_id=$1 THEN 'OWNER' ELSE COALESCE(m.role,'') END,s.status,s.backup_limit,
 s.status='RUNNING' AND s.pterodactyl_identifier IS NOT NULL
 AND NOT EXISTS(SELECT 1 FROM jobs j WHERE j.server_id=s.id AND j.status IN ('PENDING','RUNNING'))
 AND NOT EXISTS(SELECT 1 FROM subscriptions su WHERE su.server_id=s.id AND (su.status<>'ACTIVE' OR su.current_period_end<=now()))
 FROM servers s LEFT JOIN server_members m ON m.server_id=s.id AND m.user_id=$1 WHERE s.id=$2`, actor, id).Scan(&role, &state, &limit, &eligible)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !server.Allowed(role, server.Backups)) {
		return v, server.ErrNotFound
	}
	if err != nil {
		return v, err
	}
	v.CanManage = role == server.Owner
	err = s.DB.QueryRow(ctx, `SELECT enabled,hour,minute,timezone,revision,next_run,last_success FROM backup_schedules WHERE server_id=$1`, id).Scan(&v.Enabled, &v.Hour, &v.Minute, &v.Timezone, &v.Revision, &v.NextRun, &v.LastSuccess)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return v, err
	}
	rows, err := s.DB.Query(ctx, `SELECT id,scheduled_at,status,backup_uuid,error_code,completed_at FROM backup_runs WHERE server_id=$1 ORDER BY scheduled_at DESC LIMIT 20`, id)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var r Run
		if err = rows.Scan(&r.ID, &r.ScheduledAt, &r.Status, &r.BackupUUID, &r.ErrorCode, &r.CompletedAt); err != nil {
			return v, err
		}
		v.Runs = append(v.Runs, r)
	}
	if err = rows.Err(); err != nil {
		return v, err
	}
	if limit < 2 {
		v.Warning = "insufficient_allowance"
	} else if !eligible {
		v.Warning = "server_unavailable"
	}
	for _, r := range v.Runs {
		if r.Status == "NEEDS_INSPECTION" {
			v.Warning = "needs_inspection"
			break
		}
	}
	if v.Warning == "" && len(v.Runs) > 0 {
		r := v.Runs[0]
		if r.Status == "FAILED" || r.Status == "SKIPPED" {
			v.Warning = r.ErrorCode
		}
	}
	return v, nil
}
func (s *Service) Put(ctx context.Context, actor, id uuid.UUID, in Update, rid string) (View, error) {
	next, err := NextDaily(time.Now(), in.Hour, in.Minute, in.Timezone)
	if err != nil || in.Revision < 0 {
		return View{}, server.ErrInvalid
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return View{}, err
	}
	defer tx.Rollback(context.Background())
	var held bool
	if err = tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))", "server:"+id.String()).Scan(&held); err != nil {
		return View{}, err
	}
	if !held {
		return View{}, control.ErrBusy
	}
	var owner uuid.UUID
	var limit int
	err = tx.QueryRow(ctx, "SELECT owner_id,backup_limit FROM servers WHERE id=$1", id).Scan(&owner, &limit)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && owner != actor) {
		return View{}, server.ErrNotFound
	}
	if err != nil {
		return View{}, err
	}
	if in.Enabled {
		if limit < 2 {
			return View{}, ErrAllowance
		}
		if _, err = s.Control.Access(ctx, tx, actor, id, server.Backups); err != nil {
			return View{}, err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO backup_schedules(server_id,hour,minute,timezone) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, id, in.Hour, in.Minute, in.Timezone)
	if err != nil {
		return View{}, err
	}
	var due *time.Time
	if in.Enabled {
		due = &next
	}
	result, err := tx.Exec(ctx, `UPDATE backup_schedules SET enabled=$2,hour=$3,minute=$4,timezone=$5,revision=revision+1,next_run=$6,updated_at=now() WHERE server_id=$1 AND revision=$7`, id, in.Enabled, in.Hour, in.Minute, in.Timezone, due, in.Revision)
	if err != nil {
		return View{}, err
	}
	if result.RowsAffected() != 1 {
		return View{}, ErrRevision
	}
	if err = audit.Record(ctx, tx, actor, id, "backup.schedule.changed", rid, map[string]any{"enabled": in.Enabled, "timezone": in.Timezone, "hour": in.Hour, "minute": in.Minute, "revision": in.Revision + 1}); err != nil {
		return View{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return View{}, err
	}
	return s.Get(ctx, actor, id)
}
