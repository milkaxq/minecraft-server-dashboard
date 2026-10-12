package management

import (
	"context"
	"io"
	"strings"

	"github.com/google/uuid"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

type Backup struct {
	pterodactyl.Backup
	Automatic bool `json:"automatic"`
}

func (s *Service) Backups(ctx context.Context, user, id uuid.UUID) ([]Backup, error) {
	t, err := s.access(ctx, user, id, server.Backups)
	if err != nil {
		return nil, err
	}
	list, err := s.Panel.ListBackups(ctx, t.Identifier)
	if err != nil {
		return nil, control.ErrUnavailable
	}
	rows, err := s.DB.Query(ctx, "SELECT backup_uuid::text FROM backup_runs WHERE server_id=$1 AND panel_identifier=$2 AND backup_uuid IS NOT NULL", id, t.Identifier)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	owned := map[string]bool{}
	for rows.Next() {
		var bid string
		if err = rows.Scan(&bid); err != nil {
			return nil, err
		}
		owned[bid] = true
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Backup, 0, len(list))
	for _, b := range list {
		out = append(out, Backup{Backup: b, Automatic: owned[b.UUID]})
	}
	return out, nil
}
func backupIn(list []pterodactyl.Backup, id string) (pterodactyl.Backup, error) {
	for _, b := range list {
		if b.UUID == id {
			return b, nil
		}
	}
	return pterodactyl.Backup{}, server.ErrNotFound
}
func (s *Service) NewBackup(ctx context.Context, user, id, key uuid.UUID, name, rid string) (any, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 || strings.ContainsAny(name, "\r\n\x00") {
		return nil, server.ErrInvalid
	}
	return s.mutate(ctx, user, id, key, server.Backups, "backup.create", map[string]string{"name": name}, rid, func(t Target) (any, error) {
		var ambiguous bool
		if err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM management_operations WHERE server_id=$1 AND action IN ('backup.create','world.backup') AND status IN ('PENDING','UNKNOWN') AND NOT (user_id=$2 AND idempotency_key=$3))`, id, user, key).Scan(&ambiguous); err != nil {
			return nil, err
		}
		if ambiguous {
			return nil, control.ErrBusy
		}
		if err := scheduledBackupGuard(ctx, t); err != nil {
			return nil, err
		}
		list, err := s.Panel.ListBackups(ctx, t.Identifier)
		if err != nil {
			return nil, err
		}
		if len(list) >= t.BackupLimit {
			return nil, ErrLimit
		}
		b, err := s.Panel.CreateBackup(ctx, t.Identifier, name)
		if err != nil {
			return nil, err
		}
		if _, err = uuid.Parse(b.UUID); err != nil {
			return nil, control.ErrUnknown
		}
		return b, nil
	})
}
func (s *Service) ChangeBackup(ctx context.Context, user, id, key uuid.UUID, backup, action, confirmation, rid string) (any, error) {
	if _, err := uuid.Parse(backup); err != nil || (action != "restore" && action != "delete") || confirmation != backup {
		return nil, server.ErrInvalid
	}
	return s.mutate(ctx, user, id, key, server.Backups, "backup."+action, map[string]string{"backup": backup}, rid, func(t Target) (any, error) {
		list, err := s.Panel.ListBackups(ctx, t.Identifier)
		if err != nil {
			return nil, err
		}
		b, err := backupIn(list, backup)
		if err != nil {
			return nil, err
		}
		if action == "restore" {
			if !b.Successful || b.CompletedAt == nil {
				return nil, server.ErrInvalid
			}
			if err = s.offline(ctx, t); err != nil {
				return nil, err
			}
			err = s.Panel.RestoreBackup(ctx, t.Identifier, backup)
		} else {
			if b.CompletedAt == nil {
				return nil, control.ErrBusy
			}
			err = s.Panel.DeleteBackup(ctx, t.Identifier, backup)
		}
		if err != nil {
			return nil, err
		}
		return map[string]any{"status": "accepted", "action": action}, nil
	})
}
func (s *Service) BackupDownload(ctx context.Context, user, id uuid.UUID, backup string) (io.ReadCloser, int64, error) {
	if _, err := uuid.Parse(backup); err != nil {
		return nil, 0, server.ErrInvalid
	}
	t, err := s.access(ctx, user, id, server.Backups)
	if err != nil {
		return nil, 0, err
	}
	list, err := s.Panel.ListBackups(ctx, t.Identifier)
	if err != nil {
		return nil, 0, control.ErrUnavailable
	}
	b, err := backupIn(list, backup)
	if err != nil {
		return nil, 0, err
	}
	if !b.Successful || b.CompletedAt == nil {
		return nil, 0, server.ErrInvalid
	}
	r, err := s.Panel.DownloadBackup(ctx, t.Identifier, backup)
	if err != nil {
		return nil, 0, control.ErrUnavailable
	}
	return r, int64(t.DiskMB) * 1048576 * 2, nil
}

// A durable but unresolved scheduler mutation may already occupy a Panel slot.
// This check runs under the shared management server lock before creation.
func scheduledBackupGuard(ctx context.Context, t Target) error {
	var blocked bool
	err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM backup_runs WHERE server_id=$1 AND status='NEEDS_INSPECTION')
 OR EXISTS(SELECT 1 FROM backup_intents i JOIN backup_runs r ON r.id=i.run_id WHERE r.server_id=$1 AND i.status<>'CONFIRMED')`, t.ID).Scan(&blocked)
	if err != nil {
		return err
	}
	if blocked {
		return control.ErrBusy
	}
	return nil
}
