// Package control authorizes every live operation against application UUIDs.
package control

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/minecraft"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

var ErrState = errors.New("server not available for live management")
var ErrUnavailable = errors.New("game server service unavailable")
var ErrNotConfigured = errors.New("live management not configured")
var ErrBusy = errors.New("another operation is in progress")
var ErrUnknown = errors.New("operation outcome could not be confirmed")
var ErrCommand = errors.New("command not allowed for this role")
var ErrEULA = errors.New("minecraft EULA acceptance required")

type EULAChecker interface {
	Accepted(context.Context, string) (bool, error)
}

type PlayerProbe interface {
	Players(context.Context, string, int) (minecraft.Players, error)
}
type Service struct {
	DB      *pgxpool.Pool
	Panel   pterodactyl.ServerClient
	Players PlayerProbe
	EULA    EULAChecker
}
type Target struct {
	ID         uuid.UUID
	Identifier string
	Role       server.Role
	State      server.State
	Hostname   *string
	Port       *int
}
type querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Access is reused by console tickets and live revalidation. Server IDs, roles and
// Panel identifiers are resolved in PostgreSQL, never accepted from clients.
func (s Service) Access(ctx context.Context, q querier, user, id uuid.UUID, permission server.Permission) (Target, error) {
	return s.access(ctx, q, user, id, permission, "")
}

// AccessAddonRead permits inspecting configuration during voice-port setup only.
// Lifecycle jobs, revoked membership and expired subscriptions still block access.
func (s Service) AccessAddonRead(ctx context.Context, q querier, user, id uuid.UUID) (Target, error) {
	return s.access(ctx, q, user, id, server.Settings, "VOICE_PORT")
}
func (s Service) access(ctx context.Context, q querier, user, id uuid.UUID, permission server.Permission, readAction string) (Target, error) {
	var t Target
	var identifier *string
	var busy bool
	var entitled bool
	err := q.QueryRow(ctx, `SELECT s.id,s.pterodactyl_identifier,s.status,s.hostname,s.port,
 CASE WHEN s.owner_id=$1 THEN 'OWNER' ELSE COALESCE(m.role,'') END,
 EXISTS(SELECT 1 FROM jobs j WHERE j.server_id=s.id AND j.status IN ('PENDING','RUNNING') AND j.action<>$3),
 NOT EXISTS(SELECT 1 FROM subscriptions su WHERE su.server_id=s.id AND (su.status<>'ACTIVE' OR su.current_period_end<=now()))
 FROM servers s LEFT JOIN server_members m ON m.server_id=s.id AND m.user_id=$1 WHERE s.id=$2`, user, id, readAction).Scan(&t.ID, &identifier, &t.State, &t.Hostname, &t.Port, &t.Role, &busy, &entitled)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, server.ErrNotFound
	}
	if err != nil {
		return t, err
	}
	if !server.Allowed(t.Role, permission) {
		return t, server.ErrNotFound
	}
	if t.State != server.Running || identifier == nil || busy || !entitled {
		return t, ErrState
	}
	t.Identifier = *identifier
	return t, nil
}
func (s Service) Check(ctx context.Context, user, id uuid.UUID, permission server.Permission) (Target, error) {
	t, err := s.Access(ctx, s.DB, user, id, permission)
	if err == nil && permission == server.Console {
		err = s.liveReady(ctx, t)
	}
	return t, err
}

// Panel also guards asynchronous backup restoration. Never send power or commands
// while Panel is restoring, installing, transferring or suspended.
func (s Service) liveReady(ctx context.Context, t Target) error {
	if client, ok := s.Panel.(interface {
		Ready(context.Context, string) error
	}); ok {
		if err := client.Ready(ctx, t.Identifier); err != nil {
			if errors.Is(err, pterodactyl.ErrConflict) {
				return ErrState
			}
			return ErrUnavailable
		}
	}
	return nil
}

type Usage struct {
	PowerState       string   `json:"power_state"`
	CPUPercent       float64  `json:"cpu_percent"`
	MemoryBytes      int64    `json:"memory_bytes"`
	DiskBytes        int64    `json:"disk_bytes"`
	UptimeMS         int64    `json:"uptime_ms"`
	PlayerNames      []string `json:"player_names"`
	PlayersOnline    *int     `json:"players_online"`
	PlayersMax       *int     `json:"players_max"`
	PlayersAvailable bool     `json:"players_available"`
}

func (s Service) Usage(ctx context.Context, user, id uuid.UUID) (Usage, error) {
	t, err := s.Check(ctx, user, id, server.View)
	if err != nil {
		return Usage{}, err
	}
	if s.Panel == nil {
		return Usage{}, ErrNotConfigured
	}
	u, err := s.Panel.Resources(ctx, t.Identifier)
	if err != nil {
		return Usage{}, ErrUnavailable
	}
	result := Usage{PowerState: u.State, CPUPercent: u.CPUPercent, MemoryBytes: u.MemoryBytes, DiskBytes: u.DiskBytes, UptimeMS: u.UptimeMillis}
	if u.State == "running" {
		pending, err := recentShutdown(ctx, s.DB, id)
		if err != nil {
			return Usage{}, err
		}
		if pending {
			result.PowerState = "stopping"
		}
	}
	if u.State == "offline" {
		zero := 0
		result.PlayersOnline = &zero
		result.PlayersAvailable = true
	} else if s.Players != nil && t.Hostname != nil && t.Port != nil {
		p, err := s.Players.Players(ctx, *t.Hostname, *t.Port)
		if err == nil {
			result.PlayerNames = p.Names
			result.PlayersOnline = &p.Online
			result.PlayersMax = &p.Max
			result.PlayersAvailable = true
		}
	}
	return result, nil
}

// Panel may report "running" until Minecraft has finished saving its world.
// Keep the accepted shutdown visible across page refreshes for three minutes.
func recentShutdown(ctx context.Context, q querier, id uuid.UUID) (bool, error) {
	var pending bool
	err := q.QueryRow(ctx, `SELECT COALESCE((SELECT c.action IN ('stop','kill')
  AND c.status IN ('PENDING','SUCCEEDED') AND c.created_at > now()-interval '3 minutes'
 FROM control_requests c WHERE c.server_id=$1 AND c.action IN ('start','stop','restart','kill')
 ORDER BY c.created_at DESC,c.id DESC LIMIT 1),false)`, id).Scan(&pending)
	return pending, err
}
func validateCommand(role server.Role, command string) error {
	if strings.TrimSpace(command) == "" || len(command) == 0 || len(command) > 1024 || strings.ContainsAny(command, "\r\n\x00") {
		return server.ErrInvalid
	}
	for _, c := range command {
		if c < 32 || c == 127 {
			return server.ErrInvalid
		}
	}
	if role == server.Moderator {
		first := strings.ToLower(strings.TrimPrefix(strings.Fields(command)[0], "/"))
		switch first {
		case "list", "minecraft:list", "say", "minecraft:say", "help", "minecraft:help":
		default:
			return ErrCommand
		}
	}
	return nil
}
func (s Service) Power(ctx context.Context, user, id, key uuid.UUID, signal pterodactyl.PowerSignal, confirmation, requestID string) error {
	var permission server.Permission
	switch signal {
	case pterodactyl.PowerStart:
		permission = server.Start
	case pterodactyl.PowerStop:
		permission = server.Stop
	case pterodactyl.PowerRestart:
		permission = server.Restart
	case pterodactyl.PowerKill:
		permission = server.Kill
	default:
		return server.ErrInvalid
	}
	if signal == pterodactyl.PowerKill && confirmation != id.String() {
		return server.ErrInvalid
	}
	return s.perform(ctx, user, id, key, permission, string(signal), "", requestID)
}
func (s Service) Command(ctx context.Context, user, id, key uuid.UUID, command, requestID string) error {
	return s.perform(ctx, user, id, key, server.Console, "command", strings.TrimSpace(command), requestID)
}
func (s Service) perform(ctx context.Context, user, id, key uuid.UUID, permission server.Permission, action, command, requestID string) error {
	if key == uuid.Nil {
		return server.ErrInvalid
	}
	conn, err := s.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	lockKey := "server:" + id.String()
	var locked bool
	if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", lockKey).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return ErrBusy
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var ok bool
		if e := conn.QueryRow(cleanup, "SELECT pg_advisory_unlock(hashtextextended($1,0))", lockKey).Scan(&ok); e != nil || !ok {
			_ = conn.Conn().Close(cleanup)
		}
	}()
	target, err := s.Access(ctx, conn, user, id, permission)
	if err != nil {
		return err
	}
	if action == "command" {
		if err = validateCommand(target.Role, command); err != nil {
			return err
		}
	}
	if s.Panel == nil {
		return ErrNotConfigured
	}
	if err = s.liveReady(ctx, target); err != nil {
		return err
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(action+"\x00"+command)))
	var existingFingerprint, status string
	err = conn.QueryRow(ctx, "SELECT fingerprint,status FROM control_requests WHERE server_id=$1 AND user_id=$2 AND idempotency_key=$3", id, user, key).Scan(&existingFingerprint, &status)
	if err == nil {
		if existingFingerprint != fingerprint {
			return ErrBusy
		}
		if status == "SUCCEEDED" {
			return nil
		}
		return ErrUnknown
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if action == "stop" {
		pending, e := recentShutdown(ctx, conn, id)
		if e != nil {
			return e
		}
		if pending {
			return ErrBusy
		}
	}
	if action == "start" || action == "restart" {
		var worldUncertain bool
		if err = conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM management_operations WHERE server_id=$1 AND action IN ('world.reset','world.upload') AND status IN ('PENDING','UNKNOWN'))`, id).Scan(&worldUncertain); err != nil {
			return err
		}
		if worldUncertain {
			return ErrUnknown
		}
		if s.EULA == nil {
			return ErrNotConfigured
		}
		accepted, e := s.EULA.Accepted(ctx, target.Identifier)
		if e != nil {
			return ErrUnavailable
		}
		if !accepted {
			return ErrEULA
		}
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	operationID := uuid.New()
	if _, err = tx.Exec(ctx, `INSERT INTO control_requests(id,server_id,user_id,idempotency_key,action,fingerprint,status,request_id) VALUES($1,$2,$3,$4,$5,$6,'PENDING',$7)`, operationID, id, user, key, action, fingerprint, requestID); err != nil {
		return err
	}
	// Command contents may contain secrets. Save only action, length and operation ID.
	if err = audit.Record(ctx, tx, user, id, "control.requested", requestID, map[string]any{"operation_id": operationID, "action": action, "command_length": len(command)}); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	// Recheck access after recording intent; lifecycle jobs cannot run while this
	// connection holds the same server advisory lock used by the worker.
	if _, err = s.Access(ctx, conn, user, id, permission); err == nil {
		if action == "command" {
			err = s.Panel.SendCommand(ctx, target.Identifier, command)
		} else {
			err = s.Panel.Power(ctx, target.Identifier, pterodactyl.PowerSignal(action))
		}
	}
	outcome := "SUCCEEDED"
	if err != nil {
		outcome = "UNKNOWN"
		if pterodactyl.Rejected(err) || errors.Is(err, ErrState) || errors.Is(err, server.ErrNotFound) {
			outcome = "REJECTED"
		}
	}
	finishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, e := conn.Begin(finishCtx)
	if e != nil {
		return ErrUnknown
	}
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(finishCtx, "UPDATE control_requests SET status=$2,updated_at=now() WHERE id=$1", operationID, outcome); e != nil {
		return ErrUnknown
	}
	if e = audit.Record(finishCtx, tx, user, id, "control.completed", requestID, map[string]any{"operation_id": operationID, "action": action, "outcome": outcome}); e != nil {
		return ErrUnknown
	}
	if e = tx.Commit(finishCtx); e != nil {
		return ErrUnknown
	}
	if err != nil {
		return ErrUnknown
	}
	return nil
}
