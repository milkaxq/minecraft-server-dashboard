package node

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ReserveVoicePort uses the same locked pool as game provisioning. A reservation
// survives uncertain Panel outcomes and is freed only after confirmed termination.
func ReserveVoicePort(ctx context.Context, tx pgx.Tx, id uuid.UUID) (Reservation, bool, error) {
	var r Reservation
	var nodeID *uuid.UUID
	if err := tx.QueryRow(ctx, "SELECT node_id FROM servers WHERE id=$1 FOR UPDATE", id).Scan(&nodeID); err != nil {
		return r, false, err
	}
	if nodeID == nil {
		return r, false, ErrInvariant
	}
	var attempted bool
	err := tx.QueryRow(ctx, `SELECT n.id,n.pterodactyl_node_id,a.pterodactyl_allocation_id,a.hostname,a.port,p.assignment_attempted FROM server_addon_ports p JOIN node_allocations a ON a.id=p.allocation_id JOIN nodes n ON n.id=a.node_id WHERE p.server_id=$1`, id).Scan(&r.NodeID, &r.PanelNodeID, &r.PanelAllocationID, &r.Hostname, &r.Port, &attempted)
	if err == nil {
		if r.NodeID != *nodeID {
			return r, false, ErrInvariant
		}
		return r, attempted, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return r, false, err
	}
	var allocation uuid.UUID
	err = tx.QueryRow(ctx, `SELECT a.id,n.id,n.pterodactyl_node_id,a.pterodactyl_allocation_id,a.hostname,a.port FROM node_allocations a JOIN nodes n ON n.id=a.node_id WHERE a.node_id=$1 AND n.enabled AND a.enabled AND a.voice_enabled AND a.reserved_server_id IS NULL AND NOT EXISTS(SELECT 1 FROM server_addon_ports p WHERE p.allocation_id=a.id) ORDER BY a.port,a.id FOR UPDATE OF a SKIP LOCKED LIMIT 1`, nodeID).Scan(&allocation, &r.NodeID, &r.PanelNodeID, &r.PanelAllocationID, &r.Hostname, &r.Port)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, false, ErrCapacity
	}
	if err != nil {
		return r, false, err
	}
	_, err = tx.Exec(ctx, "INSERT INTO server_addon_ports(server_id,allocation_id,addon) VALUES($1,$2,'simple-voice-chat')", id, allocation)
	return r, false, err
}
