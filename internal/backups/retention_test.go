package backups

import (
	"testing"

	"minecraft-hosting/internal/pterodactyl"
)

func TestRotationPreservesLastSuccessAndManual(t *testing.T) {
	done := "2026-10-13T01:00:00Z"
	older := "2026-10-12T01:00:00Z"
	last := pterodactyl.Backup{UUID: "last", Successful: true, CompletedAt: &done}
	manual := pterodactyl.Backup{UUID: "manual", Name: "Automatic fake", Successful: true, CompletedAt: &older}
	old := pterodactyl.Backup{UUID: "old", Successful: true, CompletedAt: &older}
	for _, c := range []struct {
		name  string
		items []pterodactyl.Backup
		owned map[string]bool
		want  string
	}{
		{"manual", []pterodactyl.Backup{last, manual}, map[string]bool{"last": true}, ""},
		{"older", []pterodactyl.Backup{last, old, manual}, map[string]bool{"last": true, "old": true}, "old"},
		{"locked", []pterodactyl.Backup{last, {UUID: "old", Successful: true, CompletedAt: &older, Locked: true}}, map[string]bool{"last": true, "old": true}, ""},
		{"creating", []pterodactyl.Backup{last, {UUID: "old"}}, map[string]bool{"last": true, "old": true}, ""},
		{"failed cleanup", []pterodactyl.Backup{last, {UUID: "failed", CompletedAt: &older}}, map[string]bool{"last": true, "failed": true}, "failed"},
		{"locked latest still preserved", []pterodactyl.Backup{{UUID: "last", Successful: true, CompletedAt: &done, Locked: true}, old}, map[string]bool{"last": true, "old": true}, "old"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, ok := RotationCandidate(c.items, c.owned)
			if got != c.want || ok != (c.want != "") {
				t.Fatalf("got %q %v want %q", got, ok, c.want)
			}
		})
	}
}
