package server

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/plan"
)

type Service struct{ DB *pgxpool.Pool }

const columns = "s.id,s.owner_id,s.plan_id,s.name,s.status,s.minecraft_version,s.server_type,s.hostname,s.port,s.ram_mb,s.disk_mb,s.cpu_limit,s.backup_limit,s.database_limit,s.created_at,s.updated_at"

func scan(row pgx.Row) (Server, error) {
	var s Server
	err := row.Scan(&s.ID, &s.OwnerID, &s.PlanID, &s.Name, &s.Status, &s.MinecraftVersion, &s.ServerType, &s.Hostname, &s.Port, &s.RAMMB, &s.DiskMB, &s.CPULimit, &s.BackupLimit, &s.DatabaseLimit, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

// Create persists only a pending-payment record; it never reserves resources or provisions.
func (s Service) Create(ctx context.Context, owner uuid.UUID, input CreateInput, requestID string) (Server, error) {
	if err := input.Validate(); err != nil {
		return Server{}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Server{}, err
	}
	defer tx.Rollback(context.Background())

	result, err := s.CreateTx(ctx, tx, owner, input, requestID)
	if err != nil {
		return Server{}, err
	}
	return result, tx.Commit(ctx)
}

// CreateTx lets billing create the server and its order atomically using the same
// plan/software checks as a standalone pending request.
func (s Service) CreateTx(ctx context.Context, tx pgx.Tx, owner uuid.UUID, input CreateInput, requestID string) (Server, error) {
	if err := input.Validate(); err != nil {
		return Server{}, err
	}
	var err error
	var supported bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM minecraft_versions v JOIN software_templates t ON t.server_type=v.server_type WHERE v.server_type=$1 AND v.version=$2 AND v.enabled AND t.enabled)`, input.ServerType, input.MinecraftVersion).Scan(&supported); err != nil {
		return Server{}, err
	}
	if !supported {
		return Server{}, ErrInvalid
	}
	p, err := plan.Scan(tx.QueryRow(ctx, "SELECT "+plan.Columns+" FROM plans WHERE id=$1 AND enabled FOR SHARE", input.PlanID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Server{}, plan.ErrUnavailable
	}
	if err != nil {
		return Server{}, err
	}
	id := uuid.New()
	result, err := scan(tx.QueryRow(ctx, `INSERT INTO servers AS s (id,owner_id,plan_id,name,minecraft_version,server_type,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING `+columns, id, owner, p.ID, input.Name, input.MinecraftVersion, input.ServerType, p.RAMMB, p.DiskMB, p.CPULimit, p.BackupLimit, p.DatabaseLimit))
	if err != nil {
		return Server{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_logs(id,actor_id,server_id,action,request_id) VALUES($1,$2,$3,'server.created',$4)", uuid.New(), owner, id, requestID); err != nil {
		return Server{}, err
	}
	return result, nil
}
func (s Service) List(ctx context.Context, userID uuid.UUID) ([]Server, error) {
	rows, err := s.DB.Query(ctx, "SELECT "+columns+" FROM servers s WHERE s.owner_id=$1 OR EXISTS(SELECT 1 FROM server_members m WHERE m.server_id=s.id AND m.user_id=$1) ORDER BY s.created_at DESC,s.id", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Server{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// Unauthorized and missing servers have identical responses to avoid leaking existence.
func (s Service) Get(ctx context.Context, userID, id uuid.UUID) (Server, error) {
	v, err := scan(s.DB.QueryRow(ctx, "SELECT "+columns+" FROM servers s WHERE s.id=$2 AND (s.owner_id=$1 OR EXISTS(SELECT 1 FROM server_members m WHERE m.server_id=s.id AND m.user_id=$1))", userID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Server{}, ErrNotFound
	}
	return v, err
}
func (s Service) Authorize(ctx context.Context, userID, id uuid.UUID, permission Permission) error {
	var role Role
	err := s.DB.QueryRow(ctx, `SELECT CASE WHEN s.owner_id=$1 THEN 'OWNER' ELSE COALESCE(m.role,'') END FROM servers s LEFT JOIN server_members m ON m.server_id=s.id AND m.user_id=$1 WHERE s.id=$2`, userID, id).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !Allowed(role, permission) {
		return ErrNotFound
	}
	return nil
}

func (s Service) Role(ctx context.Context, userID, id uuid.UUID) (Role, error) {
	var role Role
	err := s.DB.QueryRow(ctx, `SELECT CASE WHEN s.owner_id=$1 THEN 'OWNER' ELSE COALESCE(m.role,'') END FROM servers s LEFT JOIN server_members m ON m.server_id=s.id AND m.user_id=$1 WHERE s.id=$2`, userID, id).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !Allowed(role, View) {
		return "", ErrNotFound
	}
	return role, err
}
func Permissions(role Role) []Permission {
	result := []Permission{}
	for _, p := range []Permission{View, Console, Start, Restart, Stop, Kill, Files, Settings, Plugins, Worlds, Backups, Billing, Members, Delete, AcceptEULA} {
		if Allowed(role, p) {
			result = append(result, p)
		}
	}
	return result
}
