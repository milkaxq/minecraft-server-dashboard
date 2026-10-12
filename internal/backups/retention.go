package backups

import (
	"time"

	"minecraft-hosting/internal/pterodactyl"
)

// RotationCandidate grants deletion only to proven UUIDs. Last-success
// protection is independent of lock state, and malformed timestamps fail closed.
func RotationCandidate(items []pterodactyl.Backup, owned map[string]bool) (string, bool) {
	var latest string
	var latestAt time.Time
	for _, b := range items {
		if !owned[b.UUID] || !b.Successful || b.CompletedAt == nil {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, *b.CompletedAt)
		if err != nil {
			return "", false
		}
		if latest == "" || at.After(latestAt) || (at.Equal(latestAt) && b.UUID < latest) {
			latest = b.UUID
			latestAt = at
		}
	}
	var candidate string
	var oldest time.Time
	var failed bool
	for _, b := range items {
		if !owned[b.UUID] || b.UUID == latest || b.Locked || b.CompletedAt == nil {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, *b.CompletedAt)
		if err != nil {
			return "", false
		}
		isFailed := !b.Successful
		if candidate == "" || (isFailed && !failed) || (isFailed == failed && (at.Before(oldest) || (at.Equal(oldest) && b.UUID < candidate))) {
			candidate = b.UUID
			oldest = at
			failed = isFailed
		}
	}
	return candidate, candidate != ""
}
