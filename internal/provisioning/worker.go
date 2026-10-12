// Package provisioning reconciles durable application intent with Pterodactyl.
package provisioning

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/node"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

var ErrAmbiguous = errors.New("panel_create_outcome_unknown")
var ErrInstallation = errors.New("panel_installation_failed")
var ErrInstalling = errors.New("panel_installation_pending")
var ErrMismatch = errors.New("panel_resource_mismatch")
var ErrSoftware = errors.New("software_configuration_unavailable")
var errUserBusy = errors.New("panel_user_operation_busy")

type Worker struct {
	DB       *pgxpool.Pool
	Panel    pterodactyl.ApplicationClient
	Log      *slog.Logger
	Verifier InstallationVerifier
}

func (w *Worker) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		worked, err := w.RunOnce(ctx)
		if err != nil {
			w.Log.Error("worker iteration failed", "category", "database")
		}
		if !worked || err != nil {
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
		}
	}
}

// RunOnce is also used by integration tests; it performs at most one claimed command.
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	lease, err := (jobs.Service{DB: w.DB}).Claim(ctx)
	if err != nil || lease == nil {
		return false, err
	}
	defer lease.Close()
	j := lease.Job
	w.Log.Info("job started", "job_id", j.ID, "server_id", j.ServerID, "request_id", j.RequestID, "action", j.Action, "attempt", j.Attempts)
	jobCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if j.Action == jobs.Provision {
		err = w.provision(jobCtx, lease)
	} else {
		err = w.lifecycle(jobCtx, lease)
	}
	if err == nil {
		w.Log.Info("job completed", "job_id", j.ID, "server_id", j.ServerID, "request_id", j.RequestID, "action", j.Action)
		return true, nil
	}
	if ctx.Err() != nil || lease.Conn.Conn().IsClosed() {
		return true, err
	}
	// Persist only controlled error codes, never Panel response bodies or secrets.
	code := errorCode(err)
	terminal := errors.Is(err, ErrAmbiguous) || errors.Is(err, ErrMismatch) || errors.Is(err, ErrSoftware) || errors.Is(err, ErrInstallation) || errors.Is(err, node.ErrInvariant) || pterodactyl.Rejected(err) || j.Attempts >= j.MaxAttempts
	w.Log.Warn("job attempt failed", "job_id", j.ID, "server_id", j.ServerID, "request_id", j.RequestID, "error_code", code, "terminal", terminal)
	finishCtx, finishCancel := context.WithTimeout(ctx, 10*time.Second)
	defer finishCancel()
	tx, e := lease.Conn.Begin(finishCtx)
	if e != nil {
		return true, e
	}
	defer tx.Rollback(context.Background())
	if terminal {
		if _, e = tx.Exec(finishCtx, "UPDATE jobs SET status='FAILED',last_error=$2,updated_at=now() WHERE id=$1", j.ID, code); e != nil {
			return true, e
		}
		if j.Action == jobs.Provision {
			if _, e = tx.Exec(finishCtx, "UPDATE servers SET status='FAILED',internal_error=$2,updated_at=now() WHERE id=$1 AND status='PROVISIONING'", j.ServerID, code); e != nil {
				return true, e
			}
		}
		if e = audit.Record(finishCtx, tx, j.ActorID, j.ServerID, "job.failed", j.RequestID, map[string]any{"job_id": j.ID, "error_code": code}); e != nil {
			return true, e
		}
	} else {
		delay := min(time.Duration(1<<min(j.Attempts, 4))*2*time.Second, 30*time.Second)
		if _, e = tx.Exec(finishCtx, "UPDATE jobs SET status='PENDING',last_error=$2,available_at=now()+$3*interval '1 second',updated_at=now() WHERE id=$1", j.ID, code, int(delay.Seconds())); e != nil {
			return true, e
		}
	}
	return true, tx.Commit(finishCtx)
}
func errorCode(err error) string {
	switch {
	case errors.Is(err, ErrAmbiguous):
		return "panel_create_outcome_unknown"
	case errors.Is(err, ErrMismatch):
		return "panel_resource_mismatch"
	case errors.Is(err, ErrSoftware):
		return "software_configuration_unavailable"
	case errors.Is(err, ErrInstallation):
		return "panel_installation_failed"
	case errors.Is(err, ErrInstalling):
		return "panel_installation_pending"
	case errors.Is(err, node.ErrCapacity):
		return "node_capacity_unavailable"
	case errors.Is(err, node.ErrInvariant):
		return "reservation_inconsistent"
	case pterodactyl.Rejected(err):
		return "panel_request_rejected"
	default:
		return "operation_retry_required"
	}
}
func (w *Worker) request(ctx context.Context, l *jobs.Lease) (pterodactyl.ServerRequest, uuid.UUID, error) {
	tx, err := l.Conn.Begin(ctx)
	if err != nil {
		return pterodactyl.ServerRequest{}, uuid.Nil, err
	}
	defer tx.Rollback(context.Background())
	var owner uuid.UUID
	var name, kind, version string
	var state server.State
	var limits pterodactyl.Limits
	var saved []byte
	err = tx.QueryRow(ctx, `SELECT owner_id,name,status,server_type,minecraft_version,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit,provisioning_request FROM servers WHERE id=$1 FOR UPDATE`, l.Job.ServerID).Scan(&owner, &name, &state, &kind, &version, &limits.RAMMB, &limits.DiskMB, &limits.CPU, &limits.Backups, &limits.Databases, &saved)
	if err != nil {
		return pterodactyl.ServerRequest{}, owner, err
	}
	if requiresReceipt(kind) && w.Verifier == nil {
		return pterodactyl.ServerRequest{}, owner, ErrSoftware
	}
	if state == server.PaymentConfirmed {
		if _, err = tx.Exec(ctx, "UPDATE servers SET status='PROVISIONING',updated_at=now() WHERE id=$1", l.Job.ServerID); err != nil {
			return pterodactyl.ServerRequest{}, owner, err
		}
		if err = audit.Record(ctx, tx, l.Job.ActorID, l.Job.ServerID, "server.provisioning", l.Job.RequestID, map[string]any{"job_id": l.Job.ID}); err != nil {
			return pterodactyl.ServerRequest{}, owner, err
		}
	} else if state != server.Provisioning {
		return pterodactyl.ServerRequest{}, owner, ErrMismatch
	}
	var req pterodactyl.ServerRequest
	if len(saved) > 0 {
		if err = json.Unmarshal(saved, &req); err != nil {
			return req, owner, ErrMismatch
		}
		if req.ExternalID != l.Job.ServerID || req.Limits != limits {
			return req, owner, ErrMismatch
		}
	} else {
		var variable string
		var environment []byte
		req = pterodactyl.ServerRequest{ExternalID: l.Job.ServerID, Name: name, Limits: limits}
		err = tx.QueryRow(ctx, `SELECT t.egg_id,COALESCE(v.docker_image,t.docker_image),t.startup,t.version_variable,(t.environment || v.environment) FROM software_templates t JOIN minecraft_versions v ON v.server_type=t.server_type WHERE t.server_type=$1 AND t.enabled AND v.enabled AND v.version=$2 FOR SHARE OF t,v`, kind, version).Scan(&req.EggID, &req.Image, &req.Startup, &variable, &environment)
		if errors.Is(err, pgx.ErrNoRows) {
			return req, owner, ErrSoftware
		}
		if err != nil {
			return req, owner, err
		}
		if err = json.Unmarshal(environment, &req.Environment); err != nil {
			return req, owner, ErrSoftware
		}
		if req.Environment == nil {
			req.Environment = map[string]string{}
		}
		req.Environment[variable] = version
	}
	reserved, err := node.Reserve(ctx, tx, l.Job.ServerID, limits.RAMMB, limits.DiskMB, limits.CPU)
	if err != nil {
		return req, owner, err
	}
	if len(saved) > 0 && (req.NodeID != reserved.PanelNodeID || req.AllocationID != reserved.PanelAllocationID) {
		return req, owner, ErrMismatch
	}
	req.NodeID = reserved.PanelNodeID
	req.AllocationID = reserved.PanelAllocationID
	data, err := json.Marshal(req)
	if err != nil {
		return req, owner, err
	}
	if _, err = tx.Exec(ctx, "UPDATE servers SET provisioning_request=$2 WHERE id=$1", l.Job.ServerID, data); err != nil {
		return req, owner, err
	}
	return req, owner, tx.Commit(ctx)
}
func (w *Worker) ensureUser(ctx context.Context, l *jobs.Lease, id uuid.UUID) (int, error) {
	key := "user:" + id.String()
	var locked bool
	if err := l.Conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", key).Scan(&locked); err != nil {
		return 0, err
	}
	if !locked {
		return 0, errUserBusy
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var ok bool
		if err := l.Conn.QueryRow(unlockCtx, "SELECT pg_advisory_unlock(hashtextextended($1,0))", key).Scan(&ok); err != nil || !ok {
			_ = l.Conn.Conn().Close(unlockCtx)
		}
	}()
	var panelID *int
	var attempted bool
	var email, name string
	if err := l.Conn.QueryRow(ctx, "SELECT pterodactyl_user_id,panel_create_attempted,email,name FROM users WHERE id=$1", id).Scan(&panelID, &attempted, &email, &name); err != nil {
		return 0, err
	}
	if panelID != nil {
		return *panelID, nil
	}
	u, err := w.Panel.FindUserByExternalID(ctx, id)
	if errors.Is(err, pterodactyl.ErrNotFound) {
		if attempted {
			return 0, ErrAmbiguous
		}
		if _, err = l.Conn.Exec(ctx, "UPDATE users SET panel_create_attempted=true WHERE id=$1", id); err != nil {
			return 0, err
		}
		u, err = w.Panel.CreateUser(ctx, pterodactyl.UserRequest{ExternalID: id, Email: email, Name: name})
		if err != nil {
			if pterodactyl.Rejected(err) {
				if _, e := l.Conn.Exec(ctx, "UPDATE users SET panel_create_attempted=false WHERE id=$1", id); e != nil {
					return 0, e
				}
			}
			return 0, err
		}
	}
	if err != nil {
		return 0, err
	}
	if u.ExternalID != id || u.ID <= 0 {
		return 0, ErrMismatch
	}
	_, err = l.Conn.Exec(ctx, "UPDATE users SET pterodactyl_user_id=$2,updated_at=now() WHERE id=$1", id, u.ID)
	return u.ID, err
}
func matches(s pterodactyl.Server, r pterodactyl.ServerRequest) bool {
	return s.ID > 0 && s.Identifier != "" && s.ExternalID == r.ExternalID && s.Request.UserID == r.UserID && s.Request.NodeID == r.NodeID && s.Request.AllocationID == r.AllocationID && s.Request.EggID == r.EggID && s.Request.Limits == r.Limits
}
func (w *Worker) provision(ctx context.Context, l *jobs.Lease) error {
	if err := w.start(ctx, l); err != nil {
		return err
	}
	req, owner, err := w.request(ctx, l)
	if err != nil {
		return err
	}
	userID, err := w.ensureUser(ctx, l, owner)
	if err != nil {
		return err
	}
	if req.UserID != 0 && req.UserID != userID {
		return ErrMismatch
	}
	req.UserID = userID
	data, _ := json.Marshal(req)
	if _, err = l.Conn.Exec(ctx, "UPDATE servers SET provisioning_request=$2 WHERE id=$1", l.Job.ServerID, data); err != nil {
		return err
	}
	remote, err := w.Panel.FindServerByExternalID(ctx, req.ExternalID)
	if errors.Is(err, pterodactyl.ErrNotFound) {
		var attempted bool
		if err = l.Conn.QueryRow(ctx, "SELECT panel_create_attempted FROM servers WHERE id=$1", req.ExternalID).Scan(&attempted); err != nil {
			return err
		}
		if attempted {
			return ErrAmbiguous
		}
		if _, err = l.Conn.Exec(ctx, "UPDATE servers SET panel_create_attempted=true WHERE id=$1", req.ExternalID); err != nil {
			return err
		}
		remote, err = w.Panel.CreateServer(ctx, req)
		if err != nil {
			if pterodactyl.Rejected(err) {
				if _, e := l.Conn.Exec(ctx, "UPDATE servers SET panel_create_attempted=false WHERE id=$1", req.ExternalID); e != nil {
					return e
				}
			}
			return err
		}
	}
	if err != nil {
		return err
	}
	if !matches(remote, req) {
		return ErrMismatch
	}
	if _, err = l.Conn.Exec(ctx, "UPDATE servers SET pterodactyl_server_id=$2,pterodactyl_identifier=$3,updated_at=now() WHERE id=$1", req.ExternalID, remote.ID, remote.Identifier); err != nil {
		return err
	}
	if remote.InstallFailed {
		if l.Job.RetryInstall && l.Job.Attempts == 1 {
			if err = w.Panel.ReinstallServer(ctx, remote.ID); err != nil {
				return err
			}
			return ErrInstalling
		}
		return ErrInstallation
	}
	if !remote.Installed {
		return ErrInstalling
	}
	if remote.Suspended {
		return ErrMismatch
	}
	var kind, version string
	if err = l.Conn.QueryRow(ctx, "SELECT server_type,minecraft_version FROM servers WHERE id=$1", l.Job.ServerID).Scan(&kind, &version); err != nil {
		return err
	}
	if requiresReceipt(kind) {
		err = w.Verifier.VerifyInstallation(ctx, remote.Identifier, kind, version, req.Environment)
		if errors.Is(err, ErrInstallation) && l.Job.RetryInstall && l.Job.Attempts == 1 {
			if err = w.Panel.ReinstallServer(ctx, remote.ID); err != nil {
				return err
			}
			return ErrInstalling
		}
		if err != nil {
			return err
		}
	}
	return w.complete(ctx, l, server.Running, false)
}
func (w *Worker) complete(ctx context.Context, l *jobs.Lease, to server.State, release bool) error {
	tx, err := l.Conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var from server.State
	if err = tx.QueryRow(ctx, "SELECT status FROM servers WHERE id=$1 FOR UPDATE", l.Job.ServerID).Scan(&from); err != nil {
		return err
	}
	if !server.CanTransition(from, to) {
		return ErrMismatch
	}
	if _, err = tx.Exec(ctx, "UPDATE servers SET status=$2,internal_error=NULL,suspension_reason=CASE WHEN $2='RUNNING' THEN NULL ELSE suspension_reason END,updated_at=now() WHERE id=$1", l.Job.ServerID, to); err != nil {
		return err
	}
	if release {
		if err = node.Release(ctx, tx, l.Job.ServerID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE servers SET pterodactyl_server_id=NULL,pterodactyl_identifier=NULL,panel_create_attempted=false WHERE id=$1", l.Job.ServerID); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE jobs SET status='SUCCEEDED',last_error=NULL,updated_at=now() WHERE id=$1", l.Job.ID); err != nil {
		return err
	}
	if err = audit.Record(ctx, tx, l.Job.ActorID, l.Job.ServerID, "server."+string(to), l.Job.RequestID, map[string]any{"job_id": l.Job.ID, "from": from, "to": to}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (w *Worker) lifecycle(ctx context.Context, l *jobs.Lease) error {
	var panelID *int
	var attempted bool
	var saved []byte
	if err := l.Conn.QueryRow(ctx, "SELECT pterodactyl_server_id,panel_create_attempted,provisioning_request FROM servers WHERE id=$1", l.Job.ServerID).Scan(&panelID, &attempted, &saved); err != nil {
		return err
	}
	remote, err := w.Panel.FindServerByExternalID(ctx, l.Job.ServerID)
	if errors.Is(err, pterodactyl.ErrNotFound) && l.Job.Action == jobs.Terminate {
		if panelID == nil && attempted {
			return ErrAmbiguous
		}
		return w.complete(ctx, l, server.Cancelled, true)
	}
	if err != nil {
		return err
	}
	var req pterodactyl.ServerRequest
	if json.Unmarshal(saved, &req) != nil || !matches(remote, req) || (panelID != nil && remote.ID != *panelID) {
		return ErrMismatch
	}
	switch l.Job.Action {
	case jobs.Suspend:
		if !remote.Suspended {
			if err = w.Panel.SuspendServer(ctx, remote.ID); err != nil {
				return err
			}
		}
		return w.complete(ctx, l, server.Suspended, false)
	case jobs.Unsuspend:
		if remote.Suspended {
			if err = w.Panel.UnsuspendServer(ctx, remote.ID); err != nil {
				return err
			}
		}
		return w.complete(ctx, l, server.Running, false)
	case jobs.Terminate:
		if err = w.Panel.DeleteServer(ctx, remote.ID); err != nil {
			return err
		}
		// A successful DELETE is not enough: verify that the external record is absent before freeing capacity.
		_, err = w.Panel.FindServerByExternalID(ctx, l.Job.ServerID)
		if !errors.Is(err, pterodactyl.ErrNotFound) {
			if err != nil {
				return err
			}
			return ErrMismatch
		}
		return w.complete(ctx, l, server.Cancelled, true)
	default:
		return ErrMismatch
	}
}

func (w *Worker) start(ctx context.Context, l *jobs.Lease) error {
	tx, err := l.Conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	tag, err := tx.Exec(ctx, "UPDATE servers SET status='PROVISIONING',updated_at=now() WHERE id=$1 AND status='PAYMENT_CONFIRMED'", l.Job.ServerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		if err = audit.Record(ctx, tx, l.Job.ActorID, l.Job.ServerID, "server.provisioning", l.Job.RequestID, map[string]any{"job_id": l.Job.ID}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
