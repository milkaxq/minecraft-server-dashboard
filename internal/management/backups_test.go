package management

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

func TestBackupDestructionRequiresExactConfirmationBeforeProcessing(t *testing.T) {
	id := uuid.NewString()
	for _, action := range []string{"restore", "delete"} {
		for _, confirmation := range []string{"", "true", "confirmed", uuid.NewString()} {
			// No database or Panel: an unconfirmed operation must stop before either
			// dependency can be used, even when the supplied backup UUID is valid.
			s := &Service{}
			_, err := s.ChangeBackup(context.Background(), uuid.New(), uuid.New(), uuid.New(), id, action, confirmation, "confirmation-test")
			if !errors.Is(err, server.ErrInvalid) {
				t.Errorf("%s accepted confirmation %q: %v", action, confirmation, err)
			}
		}
	}
}

func TestAutomaticUncertaintyBlocksManualCreation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	run := uuid.New()
	if _, err := f.db.Exec(ctx, "INSERT INTO backup_schedules(server_id,hour,minute,timezone) VALUES($1,3,0,'UTC')", f.id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(ctx, `INSERT INTO backup_runs(id,server_id,schedule_revision,scheduled_at,owner_id,panel_identifier,status) VALUES($1,$2,0,now(),$3,$4,'NEEDS_INSPECTION')`, run, f.id, f.owner, f.identifier); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.NewBackup(ctx, f.owner, f.id, uuid.New(), "Manual", "test"); !errors.Is(err, control.ErrBusy) {
		t.Fatalf("unconfirmed scheduled mutation permitted manual creation: %v", err)
	}
	if f.panel.created != 0 {
		t.Fatal("Panel mutation despite unresolved scheduler intent")
	}
}

func TestBackupHistoryUsesRecordedProvenance(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	manual, automatic := uuid.New(), uuid.New()
	done := "2026-10-01T00:00:00Z"
	f.panel.backups = []pterodactyl.Backup{{UUID: manual.String(), Name: "Daily backup forged", CompletedAt: &done}, {UUID: automatic.String(), Name: "Unrelated name", CompletedAt: &done}}
	if _, err := f.db.Exec(ctx, "INSERT INTO backup_schedules(server_id,hour,minute,timezone) VALUES($1,3,0,'UTC')", f.id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(ctx, `INSERT INTO backup_runs(id,server_id,schedule_revision,scheduled_at,owner_id,panel_identifier,status,backup_uuid) VALUES($1,$2,0,now(),$3,$4,'COMPLETED',$5)`, uuid.New(), f.id, f.owner, f.identifier, automatic); err != nil {
		t.Fatal(err)
	}
	items, err := f.s.Backups(ctx, f.owner, f.id)
	if err != nil || len(items) != 2 {
		t.Fatalf("history: %+v %v", items, err)
	}
	if items[0].Automatic || !items[1].Automatic {
		t.Fatalf("name used as provenance: %+v", items)
	}
}
