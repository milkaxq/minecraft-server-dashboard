package node

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/pterodactyl"
)

type Discovery interface {
	GetNode(context.Context, int) (pterodactyl.NodeInfo, error)
	ListAllocations(context.Context, int) ([]pterodactyl.Allocation, error)
}

// Register only imports an empty, dedicated Panel node. Resource budgets must fit
// inside Panel's physical budgets, excluding any Panel over-allocation allowance.
func Register(ctx context.Context, db *pgxpool.Pool, panel Discovery, actor uuid.UUID, panelID, ram, disk, cpu int, dedicated bool) (uuid.UUID, error) {
	if !dedicated || ram <= 0 || disk <= 0 || cpu <= 0 || panelID <= 0 {
		return uuid.Nil, fmt.Errorf("positive resource budgets and dedicated node confirmation required")
	}
	if err := (jobs.Service{DB: db}).RequireAdmin(ctx, actor); err != nil {
		return uuid.Nil, err
	}
	info, err := panel.GetNode(ctx, panelID)
	if err != nil {
		return uuid.Nil, err
	}
	if info.Maintenance || info.AllocatedRAMMB != 0 || info.AllocatedDiskMB != 0 || ram > info.RAMMB || disk > info.DiskMB {
		return uuid.Nil, fmt.Errorf("node must be empty, active, and fit the configured physical budgets")
	}
	allocations, err := panel.ListAllocations(ctx, panelID)
	if err != nil {
		return uuid.Nil, err
	}
	if len(allocations) == 0 {
		return uuid.Nil, fmt.Errorf("node requires at least one allocation")
	}
	for _, a := range allocations {
		if a.Assigned {
			return uuid.Nil, fmt.Errorf("node has externally assigned allocations")
		}
		if a.ID <= 0 || a.Hostname == "" || a.Port <= 0 || a.Port > 65535 {
			return uuid.Nil, pterodactyl.ErrInvalid
		}
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(context.Background())
	id := uuid.New()
	if _, err = tx.Exec(ctx, `INSERT INTO nodes(id,pterodactyl_node_id,name,cpu_capacity,ram_capacity_mb,disk_capacity_mb) VALUES($1,$2,$3,$4,$5,$6)`, id, panelID, info.Name, cpu, ram, disk); err != nil {
		return uuid.Nil, err
	}
	for _, a := range allocations {
		if _, err = tx.Exec(ctx, `INSERT INTO node_allocations(id,node_id,pterodactyl_allocation_id,hostname,port) VALUES($1,$2,$3,$4,$5)`, uuid.New(), id, a.ID, a.Hostname, a.Port); err != nil {
			return uuid.Nil, err
		}
	}
	// Generic audits support operator configuration without a server target.
	if err = audit.Record(ctx, tx, actor, uuid.Nil, "node.registered", uuid.NewString(), map[string]any{"node_id": id, "panel_node_id": panelID, "ram_mb": ram, "disk_mb": disk, "cpu": cpu, "dedicated": true}); err != nil {
		return uuid.Nil, err
	}
	return id, tx.Commit(ctx)
}
