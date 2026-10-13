package management

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

func TestChangeBackupDecisionEnforcesOwnedCompletedBackups(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	target, err := f.s.access(ctx, f.owner, f.id, server.Plugins)
	if err != nil {
		t.Fatal(err)
	}
	bid := uuid.New()
	completed := "2026-10-13T10:00:00Z"
	f.panel.backups = []pterodactyl.Backup{{UUID: bid.String(), Successful: true, CompletedAt: &completed}}
	if err = f.s.checkBackupDecision(ctx, target, BackupDecision{}); !errors.Is(err, ErrBackupDecision) {
		t.Fatal("implicit skip", err)
	}
	if err = f.s.checkBackupDecision(ctx, target, BackupDecision{WithoutBackup: true}); err != nil {
		t.Fatal(err)
	}
	if err = f.s.checkBackupDecision(ctx, target, BackupDecision{Backup: &bid}); err != nil {
		t.Fatal(err)
	}
	other := uuid.New()
	if err = f.s.checkBackupDecision(ctx, target, BackupDecision{Backup: &other}); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("foreign backup", err)
	}
	if err = f.s.checkBackupDecision(ctx, target, BackupDecision{Backup: &bid, WithoutBackup: true}); !errors.Is(err, ErrBackupDecision) {
		t.Fatal("ambiguous choice", err)
	}
	f.panel.backups[0].Successful = false
	if err = f.s.checkBackupDecision(ctx, target, BackupDecision{Backup: &bid}); !errors.Is(err, ErrBackupDecision) {
		t.Fatal("failed backup", err)
	}
	f.panel.backups[0].Successful = true
	f.panel.backups[0].CompletedAt = nil
	if err = f.s.checkBackupDecision(ctx, target, BackupDecision{Backup: &bid}); !errors.Is(err, ErrBackupDecision) {
		t.Fatal("pending backup", err)
	}
}
