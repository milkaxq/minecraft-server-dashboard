// Package node reserves resource budgets and network allocations under row locks.
package node

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrCapacity = errors.New("node capacity unavailable")
var ErrInvariant = errors.New("resource reservation inconsistent")

type Reservation struct {
	NodeID                         uuid.UUID
	PanelNodeID, PanelAllocationID int
	Hostname                       string
	Port                           int
}

// Reserve locks the server and validates its purchased resource snapshot before
// reserving capacity. Repeated calls return the same reservation without charging
// the node again, but cannot change the resources being reserved.
func Reserve(ctx context.Context, tx pgx.Tx, id uuid.UUID, ram, disk, cpu int) (Reservation, error) {
	var r Reservation
	if id == uuid.Nil || ram <= 0 || disk <= 0 || cpu <= 0 {
		return r, ErrInvariant
	}
	var existing *uuid.UUID
	var purchasedRAM, purchasedDisk, purchasedCPU int
	err := tx.QueryRow(ctx, "SELECT node_id,ram_mb,disk_mb,cpu_limit FROM servers WHERE id=$1 FOR UPDATE", id).Scan(&existing, &purchasedRAM, &purchasedDisk, &purchasedCPU)
	if err != nil {
		return r, err
	}
	if ram != purchasedRAM || disk != purchasedDisk || cpu != purchasedCPU {
		return r, ErrInvariant
	}
	err = tx.QueryRow(ctx, `SELECT n.id,n.pterodactyl_node_id,a.pterodactyl_allocation_id,a.hostname,a.port FROM node_allocations a JOIN nodes n ON n.id=a.node_id WHERE a.reserved_server_id=$1`, id).Scan(&r.NodeID, &r.PanelNodeID, &r.PanelAllocationID, &r.Hostname, &r.Port)
	if err == nil {
		if existing == nil || *existing != r.NodeID {
			return Reservation{}, ErrInvariant
		}
		return r, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return r, err
	}
	if existing != nil {
		return r, ErrInvariant
	}
	err = tx.QueryRow(ctx, `SELECT n.id,n.pterodactyl_node_id FROM nodes n WHERE n.enabled AND n.ram_capacity_mb-n.allocated_ram_mb >= $1 AND n.disk_capacity_mb-n.allocated_disk_mb >= $2 AND n.cpu_capacity-n.allocated_cpu >= $3
 AND EXISTS(SELECT 1 FROM node_allocations a WHERE a.node_id=n.id AND a.enabled AND a.reserved_server_id IS NULL)
 ORDER BY n.allocated_ram_mb,n.id FOR UPDATE OF n SKIP LOCKED LIMIT 1`, ram, disk, cpu).Scan(&r.NodeID, &r.PanelNodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrCapacity
	}
	if err != nil {
		return r, err
	}
	var allocation uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id,pterodactyl_allocation_id,hostname,port FROM node_allocations WHERE node_id=$1 AND enabled AND reserved_server_id IS NULL ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1`, r.NodeID).Scan(&allocation, &r.PanelAllocationID, &r.Hostname, &r.Port)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrCapacity
	}
	if err != nil {
		return r, err
	}
	if _, err = tx.Exec(ctx, "UPDATE nodes SET allocated_ram_mb=allocated_ram_mb+$2,allocated_disk_mb=allocated_disk_mb+$3,allocated_cpu=allocated_cpu+$4 WHERE id=$1", r.NodeID, ram, disk, cpu); err != nil {
		return r, err
	}
	if _, err = tx.Exec(ctx, "UPDATE node_allocations SET reserved_server_id=$2 WHERE id=$1", allocation, id); err != nil {
		return r, err
	}
	_, err = tx.Exec(ctx, "UPDATE servers SET node_id=$2,hostname=$3,port=$4,updated_at=now() WHERE id=$1", id, r.NodeID, r.Hostname, r.Port)
	return r, err
}

// Release must only run after confirmed Panel deletion (or before any remote
// creation intent). The caller holds the server row lock; repeated releases are no-ops.
func Release(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	var nodeID *uuid.UUID
	var ram, disk, cpu int
	if err := tx.QueryRow(ctx, "SELECT node_id,ram_mb,disk_mb,cpu_limit FROM servers WHERE id=$1 FOR UPDATE", id).Scan(&nodeID, &ram, &disk, &cpu); err != nil {
		return err
	}
	if nodeID == nil {
		return nil
	}
	tag, err := tx.Exec(ctx, "UPDATE nodes SET allocated_ram_mb=allocated_ram_mb-$2,allocated_disk_mb=allocated_disk_mb-$3,allocated_cpu=allocated_cpu-$4 WHERE id=$1", nodeID, ram, disk, cpu)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrInvariant
	}
	tag, err = tx.Exec(ctx, "UPDATE node_allocations SET reserved_server_id=NULL WHERE reserved_server_id=$1 AND node_id=$2", id, nodeID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrInvariant
	}
	_, err = tx.Exec(ctx, "UPDATE servers SET node_id=NULL,hostname=NULL,port=NULL,provisioning_request=NULL,updated_at=now() WHERE id=$1", id)
	return err
}
