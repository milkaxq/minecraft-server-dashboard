package management

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

func TestStoragePermissionCategoriesAndSymlinkBoundary(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	for _, user := range []uuid.UUID{f.other, f.mod} {
		if _, err := f.s.Storage(ctx, user, f.id); !errors.Is(err, server.ErrNotFound) {
			t.Fatal(err)
		}
	}
	if f.panel.calls != 0 {
		t.Fatal("unauthorized scan reached Panel")
	}
	f.panel.folders["/world"] = true
	f.panel.folders["/plugins"] = true
	f.panel.folders["/linked"] = true
	f.panel.symlinks["/linked"] = true
	f.panel.files["/world/level.dat"] = []byte("world")
	f.panel.files["/plugins/addon.jar"] = []byte("addon-data")
	f.panel.files["/linked/private"] = []byte("must not follow")
	f.panel.backups = []pterodactyl.Backup{{UUID: uuid.NewString(), Bytes: 123}}
	out, err := f.s.Storage(ctx, f.owner, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Partial || out.LimitBytes != 10240*1048576 || out.BackupBytes != 123 || out.BackupCount != 1 {
		t.Fatal(out)
	}
	if out.Categories[0].Bytes != 5 || out.Categories[1].Bytes != 10 {
		t.Fatal("bad categories", out.Categories)
	}
}

func TestActivityAndNotificationsNeverLeakMetadataAndReadsArePerMember(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := server.Service{DB: f.db}
	event := uuid.New()
	_, err := f.db.Exec(ctx, `INSERT INTO audit_logs(id,actor_id,server_id,action,request_id,metadata) VALUES($1,$2,$3,'server.RUNNING','private-reference','{"password":"secret","command":"private-command"}')`, event, f.owner, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Activity(ctx, f.other, f.id, nil); !errors.Is(err, server.ErrNotFound) {
		t.Fatal(err)
	}
	items, err := svc.Activity(ctx, f.mod, f.id, nil)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	encoded, _ := json.Marshal(items)
	for _, secret := range []string{"private-reference", "secret", "private-command", "@example.com"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("private audit data leaked")
		}
	}
	notices, err := svc.Notifications(ctx, f.mod)
	if err != nil || len(notices) != 1 || notices[0].Title != "Your server is ready" {
		t.Fatal(notices, err)
	}
	if err = svc.ReadNotification(ctx, f.other, event); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("foreign notification read", err)
	}
	for i := 0; i < 2; i++ {
		if err = svc.ReadNotification(ctx, f.mod, event); err != nil {
			t.Fatal("read replay", err)
		}
	}
	notices, err = svc.Notifications(ctx, f.mod)
	if err != nil || len(notices) != 0 {
		t.Fatal(notices, err)
	}
	notices, err = svc.Notifications(ctx, f.owner)
	if err != nil || len(notices) != 1 {
		t.Fatal("one member hid another member's alert", notices, err)
	}
	_, err = f.db.Exec(ctx, "DELETE FROM server_members WHERE server_id=$1 AND user_id=$2", f.id, f.mod)
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.ReadNotification(ctx, f.mod, event); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("revoked member read", err)
	}
}
