package server

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Notification struct {
	ID         uuid.UUID `json:"id"`
	ServerID   uuid.UUID `json:"server_id"`
	ServerName string    `json:"server_name"`
	Title      string    `json:"title"`
	CreatedAt  time.Time `json:"created_at"`
}

func notificationTitle(action string) string {
	switch action {
	case "server.RUNNING":
		return "Your server is ready"
	case "server.FAILED":
		return "Server setup needs attention"
	case "server.SUSPENDED":
		return "Your server was suspended"
	case "backup.automatic.FAILED":
		return "An automatic backup failed"
	case "backup.automatic.NEEDS_INSPECTION":
		return "An automatic backup needs inspection"
	case "backup.automatic.SKIPPED":
		return "An automatic backup was skipped"
	}
	return "Server needs attention"
}

// Notices are sourced from durable lifecycle events. Reads are individual and
// idempotent; neither API returns private job errors or raw audit metadata.
func (s Service) Notifications(ctx context.Context, user uuid.UUID) ([]Notification, error) {
	rows, err := s.DB.Query(ctx, `SELECT a.id,s.id,s.name,a.action,a.created_at FROM audit_logs a JOIN servers s ON s.id=a.server_id
 WHERE a.created_at>now()-interval '30 days' AND a.action IN ('server.RUNNING','server.FAILED','server.SUSPENDED','backup.automatic.FAILED','backup.automatic.NEEDS_INSPECTION','backup.automatic.SKIPPED')
 AND NOT EXISTS(SELECT 1 FROM notification_reads n WHERE n.user_id=$1 AND n.audit_id=a.id)
 AND (s.owner_id=$1 OR EXISTS(SELECT 1 FROM server_members m WHERE m.server_id=s.id AND m.user_id=$1))
 AND s.status<>'CANCELLED' ORDER BY a.created_at DESC,a.id DESC LIMIT 30`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var item Notification
		var action string
		if err = rows.Scan(&item.ID, &item.ServerID, &item.ServerName, &action, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.Title = notificationTitle(action)
		out = append(out, item)
	}
	return out, rows.Err()
}
func (s Service) ReadNotification(ctx context.Context, user, id uuid.UUID) error {
	tag, err := s.DB.Exec(ctx, `INSERT INTO notification_reads(user_id,audit_id) SELECT $1,a.id FROM audit_logs a JOIN servers s ON s.id=a.server_id
 WHERE a.id=$2 AND a.action IN ('server.RUNNING','server.FAILED','server.SUSPENDED','backup.automatic.FAILED','backup.automatic.NEEDS_INSPECTION','backup.automatic.SKIPPED')
 AND (s.owner_id=$1 OR EXISTS(SELECT 1 FROM server_members m WHERE m.server_id=s.id AND m.user_id=$1))
 ON CONFLICT(user_id,audit_id) DO UPDATE SET read_at=notification_reads.read_at`, user, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
