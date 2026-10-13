package provisioning

import (
	"context"
	"slices"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/node"
	"minecraft-hosting/internal/pterodactyl"
)

func (w *Worker) voicePort(ctx context.Context, l *jobs.Lease) error {
	network, ok := w.Panel.(pterodactyl.AllocationManager)
	if !ok {
		return ErrSoftware
	}
	controls := control.Service{DB: w.DB}
	target, err := controls.AccessAddonRead(ctx, l.Conn, l.Job.ActorID, l.Job.ServerID)
	if err != nil {
		return ErrMismatch
	}
	var panelID int
	var limits pterodactyl.Limits
	if err = l.Conn.QueryRow(ctx, `SELECT pterodactyl_server_id,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit FROM servers WHERE id=$1`, l.Job.ServerID).Scan(&panelID, &limits.RAMMB, &limits.DiskMB, &limits.CPU, &limits.Backups, &limits.Databases); err != nil {
		return err
	}
	remote, err := w.Panel.GetServer(ctx, panelID)
	if err != nil {
		return err
	}
	if remote.ExternalID != l.Job.ServerID || remote.Identifier != target.Identifier || remote.Request.Limits != limits || remote.Suspended || !remote.Installed || remote.InstallFailed {
		return ErrMismatch
	}
	var primary int
	if err = l.Conn.QueryRow(ctx, "SELECT pterodactyl_allocation_id FROM node_allocations WHERE reserved_server_id=$1", l.Job.ServerID).Scan(&primary); err != nil {
		return node.ErrInvariant
	}
	if primary != remote.Request.AllocationID {
		return ErrMismatch
	}
	tx, err := l.Conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	reservation, attempted, err := node.ReserveVoicePort(ctx, tx, l.Job.ServerID)
	if err != nil {
		return err
	}
	if reservation.PanelNodeID != remote.Request.NodeID {
		return ErrMismatch
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	assigned, err := network.ServerAllocations(ctx, panelID)
	if err != nil {
		return err
	}
	if !slices.Contains(assigned, reservation.PanelAllocationID) {
		// An earlier HTTP request may still be in flight. Keep its reservation and
		// require operator reconciliation instead of issuing an ambiguous write twice.
		if attempted {
			return ErrAllocation
		}
		if _, err = l.Conn.Exec(ctx, "UPDATE server_addon_ports SET assignment_attempted=true,status='RESERVED',updated_at=now() WHERE server_id=$1", l.Job.ServerID); err != nil {
			return err
		}
		// This is the last step before the HTTP mutation, after all network reads.
		if _, err = controls.AccessAddonRead(ctx, l.Conn, l.Job.ActorID, l.Job.ServerID); err != nil {
			if _, resetErr := l.Conn.Exec(ctx, "UPDATE server_addon_ports SET assignment_attempted=false WHERE server_id=$1", l.Job.ServerID); resetErr != nil {
				return ErrAllocation
			}
			return ErrMismatch
		}
		err = network.AssignAllocation(ctx, pterodactyl.AdditionalAllocationRequest{ServerID: panelID, PrimaryAllocationID: remote.Request.AllocationID, AdditionalAllocationID: reservation.PanelAllocationID, Limits: limits})
		if err != nil {
			if pterodactyl.Rejected(err) {
				_, e := l.Conn.Exec(ctx, "UPDATE server_addon_ports SET assignment_attempted=false WHERE server_id=$1", l.Job.ServerID)
				if e != nil {
					return ErrAllocation
				}
				return err
			}
			return ErrAllocation
		}
		assigned, err = network.ServerAllocations(ctx, panelID)
		if err != nil || !slices.Contains(assigned, reservation.PanelAllocationID) {
			return ErrAllocation
		}
	}
	finish, err := l.Conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer finish.Rollback(context.Background())
	if _, err = finish.Exec(ctx, "UPDATE server_addon_ports SET status='READY',updated_at=now() WHERE server_id=$1", l.Job.ServerID); err != nil {
		return err
	}
	if _, err = finish.Exec(ctx, "UPDATE jobs SET status='SUCCEEDED',last_error=NULL,updated_at=now() WHERE id=$1", l.Job.ID); err != nil {
		return err
	}
	if err = audit.Record(ctx, finish, l.Job.ActorID, l.Job.ServerID, "addon.port_ready", l.Job.RequestID, map[string]any{"job_id": l.Job.ID, "addon": "simple-voice-chat", "port": reservation.Port}); err != nil {
		return err
	}
	return finish.Commit(ctx)
}
