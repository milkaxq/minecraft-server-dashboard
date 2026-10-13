package server

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Activity deliberately omits audit metadata, commands, paths and email addresses.
type Activity struct {
	ID        uuid.UUID `json:"id"`
	Actor     string    `json:"actor"`
	Title     string    `json:"title"`
	Outcome   string    `json:"outcome,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func activityTitle(action, operation string) string {
	labels := map[string]string{
		"server.created": "Server requested", "server.RUNNING": "Server ready",
		"server.SUSPENDED": "Server suspended", "server.CANCELLED": "Server terminated",
		"server.FAILED": "Server setup failed", "server.PROVISIONING": "Server setup started",
	}
	if title := labels[action]; title != "" {
		return title
	}
	if action == "control.completed" {
		switch operation {
		case "start":
			return "Start requested"
		case "stop":
			return "Stop requested"
		case "restart":
			return "Restart requested"
		case "kill":
			return "Force stop requested"
		case "command":
			return "Console command sent"
		}
	}
	if action == "management.completed" {
		switch operation {
		case "catalog.install":
			return "Catalog addon installed or updated"
		case "backup.create":
			return "Backup requested"
		case "backup.restore":
			return "Backup restore requested"
		case "backup.delete":
			return "Backup deleted"
		case "settings.update", "settings.write", "settings.save":
			return "Minecraft settings changed"
		case "plugin.upload", "mod.upload":
			return "Addon uploaded"
		case "plugin.delete", "mod.delete":
			return "Addon deleted"
		case "world.reset":
			return "World reset requested"
		case "world.upload":
			return "World uploaded"
		case "eula.accept":
			return "Minecraft agreement accepted"
		}
		return "Server configuration action"
	}
	return "Server activity"
}

// Activity returns the latest bounded page. Authorization is also embedded in the
// query so a member removed between checks cannot read the audit history.
func (s Service) Activity(ctx context.Context, user, id uuid.UUID, before *time.Time) ([]Activity, error) {
	if err := s.Authorize(ctx, user, id, View); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `SELECT a.id,COALESCE(u.name,'Hosting system'),a.action,
 COALESCE(a.metadata->>'action',''),COALESCE(a.metadata->>'outcome',''),a.created_at
 FROM audit_logs a LEFT JOIN users u ON u.id=a.actor_id
 WHERE a.server_id=$2 AND ($3::timestamptz IS NULL OR a.created_at<$3)
 AND (a.action LIKE 'server.%' OR a.action IN ('control.completed','management.completed'))
 AND EXISTS(SELECT 1 FROM servers s WHERE s.id=a.server_id AND (s.owner_id=$1 OR EXISTS(SELECT 1 FROM server_members m WHERE m.server_id=s.id AND m.user_id=$1)))
 ORDER BY a.created_at DESC,a.id DESC LIMIT 50`, user, id, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Activity{}
	for rows.Next() {
		var item Activity
		var action, operation, outcome string
		if err = rows.Scan(&item.ID, &item.Actor, &action, &operation, &outcome, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.Title = activityTitle(action, operation)
		switch outcome {
		case "SUCCEEDED", "REJECTED", "UNKNOWN":
			item.Outcome = outcome
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
