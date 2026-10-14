// Package admin exposes explicit, audited operator capabilities with bounded lists.
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/billing"
	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/plan"
	"minecraft-hosting/internal/server"
)

var ErrConflict = errors.New("plan name already exists")

type Service struct{ DB *pgxpool.Pool }

func (s Service) Require(ctx context.Context, user uuid.UUID) error {
	return (jobs.Service{DB: s.DB}).RequireAdmin(ctx, user)
}
func (s Service) Plans(ctx context.Context, user uuid.UUID) ([]plan.Plan, error) {
	if err := s.Require(ctx, user); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, "SELECT "+plan.Columns+" FROM plans ORDER BY created_at DESC,id LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []plan.Plan{}
	for rows.Next() {
		v, e := plan.Scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func validatePlan(p *plan.Plan) error {
	p.Name = strings.TrimSpace(p.Name)
	p.Description = strings.TrimSpace(p.Description)
	if len(p.Name) < 1 || len(p.Name) > 80 || len(p.Description) > 1000 || p.PriceCents < 0 || p.PriceCents > 100000000 || len(p.Currency) != 3 || p.Currency != strings.ToUpper(p.Currency) || (p.BillingPeriod != "MONTHLY" && p.BillingPeriod != "YEARLY") || p.RAMMB < 512 || p.RAMMB > 131072 || p.DiskMB < 1024 || p.DiskMB > 2097152 || p.CPULimit < 1 || p.CPULimit > 10000 || p.BackupLimit < 0 || p.BackupLimit > 100 || p.DatabaseLimit < 0 || p.DatabaseLimit > 100 || p.PlayerRecommendation < 1 || p.PlayerRecommendation > 1000 {
		return server.ErrInvalid
	}
	for _, c := range p.Currency {
		if c < 'A' || c > 'Z' {
			return server.ErrInvalid
		}
	}
	for _, c := range p.Name {
		if c < 32 || c == 127 {
			return server.ErrInvalid
		}
	}
	return nil
}
func (s Service) SavePlan(ctx context.Context, user, id uuid.UUID, p plan.Plan, rid string) (plan.Plan, error) {
	if err := s.Require(ctx, user); err != nil {
		return p, err
	}
	if err := validatePlan(&p); err != nil {
		return p, err
	}
	if p.ID != uuid.Nil && p.ID != id {
		return p, server.ErrInvalid
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return p, err
	}
	defer tx.Rollback(context.Background())
	var enabled bool
	if err = tx.QueryRow(ctx, "SELECT is_admin FROM users WHERE id=$1 AND verified_at IS NOT NULL FOR SHARE", user).Scan(&enabled); err != nil || !enabled {
		return p, jobs.ErrForbidden
	}
	creating := id == uuid.Nil
	if creating {
		id = uuid.New()
	}
	var result plan.Plan
	if creating {
		result, err = plan.Scan(tx.QueryRow(ctx, `INSERT INTO plans(id,name,description,price_cents,currency,billing_period,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit,player_recommendation,enabled) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING `+plan.Columns, id, p.Name, p.Description, p.PriceCents, p.Currency, p.BillingPeriod, p.RAMMB, p.DiskMB, p.CPULimit, p.BackupLimit, p.DatabaseLimit, p.PlayerRecommendation, p.Enabled))
	} else {
		result, err = plan.Scan(tx.QueryRow(ctx, `UPDATE plans SET name=$2,description=$3,price_cents=$4,currency=$5,billing_period=$6,ram_mb=$7,disk_mb=$8,cpu_limit=$9,backup_limit=$10,database_limit=$11,player_recommendation=$12,enabled=$13,updated_at=now() WHERE id=$1 RETURNING `+plan.Columns, id, p.Name, p.Description, p.PriceCents, p.Currency, p.BillingPeriod, p.RAMMB, p.DiskMB, p.CPULimit, p.BackupLimit, p.DatabaseLimit, p.PlayerRecommendation, p.Enabled))
	}
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) && databaseError.Code == "23505" {
		return p, ErrConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return p, server.ErrNotFound
	}
	if err != nil {
		return p, err
	}
	action := "plan.updated"
	if creating {
		action = "plan.created"
	}
	if err = audit.Record(ctx, tx, user, uuid.Nil, action, rid, map[string]any{"plan_id": id, "enabled": p.Enabled, "price_cents": p.PriceCents, "ram_mb": p.RAMMB, "disk_mb": p.DiskMB, "cpu_limit": p.CPULimit, "backup_limit": p.BackupLimit, "database_limit": p.DatabaseLimit}); err != nil {
		return p, err
	}
	return result, tx.Commit(ctx)
}

type User struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Email      string     `json:"email"`
	IsAdmin    bool       `json:"is_admin"`
	VerifiedAt *time.Time `json:"verified_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (s Service) Users(ctx context.Context, user uuid.UUID, offset int) ([]User, error) {
	if err := s.Require(ctx, user); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, "SELECT id,name,email,is_admin,verified_at,created_at FROM users ORDER BY created_at DESC,id LIMIT 50 OFFSET $1", offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var v User
		if err = rows.Scan(&v.ID, &v.Name, &v.Email, &v.IsAdmin, &v.VerifiedAt, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type Server struct {
	server.Server
	OwnerEmail       string     `json:"owner_email"`
	PlanName         string     `json:"plan_name"`
	SuspensionReason *string    `json:"suspension_reason"`
	JobID            *uuid.UUID `json:"job_id"`
	JobAction        *string    `json:"job_action"`
	JobStatus        *string    `json:"job_status"`
	JobError         *string    `json:"job_error"`
}

func (s Service) Servers(ctx context.Context, user uuid.UUID, offset int) ([]Server, error) {
	if err := s.Require(ctx, user); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `SELECT s.id,s.owner_id,s.plan_id,s.name,s.status,s.minecraft_version,s.server_type,s.hostname,s.port,s.ram_mb,s.disk_mb,s.cpu_limit,s.backup_limit,s.database_limit,s.created_at,s.updated_at,u.email,p.name,s.suspension_reason,j.id,j.action,j.status,j.last_error FROM servers s JOIN users u ON u.id=s.owner_id JOIN plans p ON p.id=s.plan_id LEFT JOIN LATERAL (SELECT id,action,status,last_error FROM jobs WHERE server_id=s.id ORDER BY created_at DESC,id DESC LIMIT 1) j ON true ORDER BY s.created_at DESC,s.id LIMIT 50 OFFSET $1`, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Server{}
	for rows.Next() {
		var v Server
		if err = rows.Scan(&v.ID, &v.OwnerID, &v.PlanID, &v.Name, &v.Status, &v.MinecraftVersion, &v.ServerType, &v.Hostname, &v.Port, &v.RAMMB, &v.DiskMB, &v.CPULimit, &v.BackupLimit, &v.DatabaseLimit, &v.CreatedAt, &v.UpdatedAt, &v.OwnerEmail, &v.PlanName, &v.SuspensionReason, &v.JobID, &v.JobAction, &v.JobStatus, &v.JobError); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s Service) Orders(ctx context.Context, user uuid.UUID, offset int) ([]billing.Order, error) {
	if err := s.Require(ctx, user); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, "SELECT "+billing.OrderColumns+" FROM orders ORDER BY created_at DESC,id DESC LIMIT 50 OFFSET $1", offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []billing.Order{}
	for rows.Next() {
		o, e := billing.ScanOrder(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

type Node struct {
	ID                   uuid.UUID `json:"id"`
	Name                 string    `json:"name"`
	CPU                  int       `json:"cpu_capacity"`
	RAM                  int       `json:"ram_capacity_mb"`
	Disk                 int       `json:"disk_capacity_mb"`
	AllocatedCPU         int       `json:"allocated_cpu"`
	AllocatedRAM         int       `json:"allocated_ram_mb"`
	AllocatedDisk        int       `json:"allocated_disk_mb"`
	AvailableAllocations int       `json:"available_allocations"`
	Enabled              bool      `json:"enabled"`
}

func (s Service) Nodes(ctx context.Context, user uuid.UUID) ([]Node, error) {
	if err := s.Require(ctx, user); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, "SELECT id,name,cpu_capacity,ram_capacity_mb,disk_capacity_mb,allocated_cpu,allocated_ram_mb,allocated_disk_mb,enabled,(SELECT count(*) FROM node_allocations WHERE node_id=nodes.id AND enabled AND reserved_server_id IS NULL AND NOT EXISTS(SELECT 1 FROM server_addon_ports extra WHERE extra.allocation_id=node_allocations.id)) FROM nodes ORDER BY name,id LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Node{}
	for rows.Next() {
		var v Node
		if err = rows.Scan(&v.ID, &v.Name, &v.CPU, &v.RAM, &v.Disk, &v.AllocatedCPU, &v.AllocatedRAM, &v.AllocatedDisk, &v.Enabled, &v.AvailableAllocations); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type Audit struct {
	ID        uuid.UUID       `json:"id"`
	ActorID   *uuid.UUID      `json:"actor_id"`
	ServerID  *uuid.UUID      `json:"server_id"`
	Action    string          `json:"action"`
	RequestID string          `json:"request_id"`
	Metadata  json.RawMessage `json:"metadata"`
	CreatedAt time.Time       `json:"created_at"`
}

func (s Service) Audit(ctx context.Context, user uuid.UUID, offset int) ([]Audit, error) {
	if err := s.Require(ctx, user); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, "SELECT id,actor_id,server_id,action,request_id,metadata,created_at FROM audit_logs ORDER BY created_at DESC,id DESC LIMIT 50 OFFSET $1", offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Audit{}
	for rows.Next() {
		var v Audit
		if err = rows.Scan(&v.ID, &v.ActorID, &v.ServerID, &v.Action, &v.RequestID, &v.Metadata, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
