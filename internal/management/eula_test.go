package management

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

func TestEULAProperties(t *testing.T) {
	for _, tc := range []struct {
		content  string
		accepted bool
	}{
		{"", false}, {"# eula=true\neula=false\n", false},
		{"eula=true\n", true}, {"eula: true\n", true},
		{"eula=true\neula=false\n", false}, {"not-eula=true\n", false},
		{"eula=tr\\\n ue\n", true},
	} {
		if got := eulaAccepted(tc.content); got != tc.accepted {
			t.Errorf("%q: got %v", tc.content, got)
		}
	}
}

func TestEULAAuthorizationAcceptanceAndReplay(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.s.Control.EULA = f.s
	before := f.panel.calls
	if _, err := f.s.EULA(ctx, f.other, f.id); !errors.Is(err, server.ErrNotFound) {
		t.Fatal(err)
	}
	for _, user := range []uuid.UUID{f.other, f.admin, f.mod} {
		if _, err := f.s.AcceptEULA(ctx, user, f.id, uuid.New(), true, "test"); !errors.Is(err, server.ErrNotFound) {
			t.Fatal("non-owner accepted", err)
		}
	}
	if before != f.panel.calls {
		t.Fatal("unauthorized request reached Panel")
	}
	status, err := f.s.EULA(ctx, f.owner, f.id)
	if err != nil || status.Accepted || !status.CanAccept {
		t.Fatal(status, err)
	}
	status, err = f.s.EULA(ctx, f.mod, f.id)
	if err != nil || status.Accepted || status.CanAccept {
		t.Fatal(status, err)
	}
	if _, err := f.s.AcceptEULA(ctx, f.owner, f.id, uuid.New(), false, "test"); !errors.Is(err, server.ErrInvalid) {
		t.Fatal("implicit acceptance allowed", err)
	}
	key := uuid.New()
	for range 2 {
		if _, err := f.s.AcceptEULA(ctx, f.owner, f.id, key, true, "owner-confirmed"); err != nil {
			t.Fatal(err)
		}
	}
	accepted, err := f.s.Accepted(ctx, f.identifier)
	if err != nil || !accepted {
		t.Fatal(accepted, err)
	}
	var count int
	if err := f.db.QueryRow(ctx, "SELECT count(*) FROM management_operations WHERE server_id=$1 AND action='eula.accept' AND status='SUCCEEDED'", f.id).Scan(&count); err != nil || count != 1 {
		t.Fatal("acceptance replay created duplicate intent", count, err)
	}
	if err := f.db.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE server_id=$1 AND action='management.requested' AND metadata->>'action'='eula.accept'", f.id).Scan(&count); err != nil || count != 1 {
		t.Fatal("missing acceptance audit", count, err)
	}
	f.panel.files["/eula.txt"] = []byte("eula=false\n")
	accepted, err = f.s.Accepted(ctx, f.identifier)
	if err != nil || accepted {
		t.Fatal("acceptance was cached after file changed", accepted, err)
	}
}

func TestEULASymlinkRejected(t *testing.T) {
	f := setup(t)
	f.panel.files["/eula.txt"] = []byte("eula=true\n")
	f.panel.symlinks["/eula.txt"] = true
	if _, err := f.s.AcceptEULA(context.Background(), f.owner, f.id, uuid.New(), true, "test"); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestEULAAcceptanceRequiresOfflineServer(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if err := f.panel.Power(ctx, f.identifier, pterodactyl.PowerStart); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AcceptEULA(ctx, f.owner, f.id, uuid.New(), true, "test"); !errors.Is(err, ErrOffline) {
		t.Fatal("agreement file changed while running", err)
	}
	if _, exists := f.panel.files["/eula.txt"]; exists {
		t.Fatal("agreement was written")
	}
}
