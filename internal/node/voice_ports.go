package node

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/jobs"
)

// EnableVoicePorts is an operator action: UDP reachability must be confirmed on
// the node and provider firewall first. Customer jobs only select this safe pool.
func EnableVoicePorts(ctx context.Context, db *pgxpool.Pool, actor, node uuid.UUID, ports []int, udpReady bool) error {
	if !udpReady || node == uuid.Nil || len(ports) == 0 || len(ports) > 64 {
		return fmt.Errorf("node, ports and UDP readiness confirmation required")
	}
	ports = slices.Clone(ports)
	slices.Sort(ports)
	ports = slices.Compact(ports)
	for _, port := range ports {
		if port < 1024 || port > 65535 {
			return fmt.Errorf("only unprivileged ports are allowed")
		}
	}
	if err := (jobs.Service{DB: db}).RequireAdmin(ctx, actor); err != nil {
		return err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	tag, err := tx.Exec(ctx, `UPDATE node_allocations a SET voice_enabled=true WHERE a.node_id=$1 AND a.port=ANY($2) AND a.enabled AND a.reserved_server_id IS NULL AND NOT EXISTS(SELECT 1 FROM server_addon_ports p WHERE p.allocation_id=a.id)`, node, ports)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != int64(len(ports)) {
		return fmt.Errorf("ports must identify distinct unreserved allocations on this node")
	}
	if err = audit.Record(ctx, tx, actor, uuid.Nil, "node.voice_ports_enabled", uuid.NewString(), map[string]any{"node_id": node, "ports": ports, "udp_ready": true}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
