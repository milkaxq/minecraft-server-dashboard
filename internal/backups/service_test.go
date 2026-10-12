package backups

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/server"
	"minecraft-hosting/internal/testutil"
)

type fixture struct {
	db                           *pgxpool.Pool
	s                            *Service
	owner, admin, mod, other, id uuid.UUID
}

func setup(t *testing.T) fixture {
	t.Helper()
	db := testutil.Database(t)
	ctx := context.Background()
	f := fixture{db: db, owner: uuid.New(), admin: uuid.New(), mod: uuid.New(), other: uuid.New(), id: uuid.New()}
	for _, u := range []uuid.UUID{f.owner, f.admin, f.mod, f.other} {
		if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at) VALUES($1,$2,'unused','Player',now())", u, u.String()+"@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	node := uuid.New()
	if _, err := db.Exec(ctx, "INSERT INTO nodes(id,pterodactyl_node_id,name,cpu_capacity,ram_capacity_mb,disk_capacity_mb) VALUES($1,1,'Test',100,2048,10240)", node); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO servers(id,owner_id,node_id,plan_id,pterodactyl_server_id,pterodactyl_identifier,name,status,minecraft_version,server_type,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit) VALUES($1,$2,$3,'10000000-0000-4000-8000-000000000001',1,'abcdef12','World','RUNNING','1.21.1','PAPER',2048,10240,100,2,0)`, f.id, f.owner, node); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO server_members(server_id,user_id,role) VALUES($1,$2,'ADMIN'),($1,$3,'MODERATOR')", f.id, f.admin, f.mod); err != nil {
		t.Fatal(err)
	}
	f.s = &Service{Available: true, DB: db, Control: &control.Service{DB: db}}
	return f
}
func TestSchedulePermissionsAndRevision(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	in := Update{Enabled: true, Hour: 3, Minute: 15, Timezone: "Asia/Ashgabat"}
	for _, u := range []uuid.UUID{f.other, f.mod} {
		if _, err := f.s.Get(ctx, u, f.id); !errors.Is(err, server.ErrNotFound) {
			t.Fatalf("unauthorized read: %v", err)
		}
	}
	v, err := f.s.Get(ctx, f.admin, f.id)
	if err != nil || v.CanManage || v.Enabled || v.Revision != 0 {
		t.Fatalf("admin view: %+v %v", v, err)
	}
	for _, u := range []uuid.UUID{f.other, f.admin, f.mod} {
		if _, err := f.s.Put(ctx, u, f.id, in, "test"); !errors.Is(err, server.ErrNotFound) {
			t.Fatalf("unauthorized write: %v", err)
		}
	}
	v, err = f.s.Put(ctx, f.owner, f.id, in, "test")
	if err != nil || !v.Enabled || v.NextRun == nil || v.Revision != 1 {
		t.Fatalf("enable: %+v %v", v, err)
	}
	if _, err = f.s.Put(ctx, f.owner, f.id, in, "test"); !errors.Is(err, ErrRevision) {
		t.Fatalf("stale update: %v", err)
	}
	if _, err = f.db.Exec(ctx, "UPDATE servers SET status='SUSPENDED' WHERE id=$1", f.id); err != nil {
		t.Fatal(err)
	}
	v, err = f.s.Get(ctx, f.owner, f.id)
	if err != nil || v.Warning != "server_unavailable" {
		t.Fatalf("suspended view: %+v %v", v, err)
	}
	in.Enabled = false
	in.Revision = 1
	v, err = f.s.Put(ctx, f.owner, f.id, in, "test")
	if err != nil || v.Enabled || v.NextRun != nil || v.Revision != 2 {
		t.Fatalf("disable: %+v %v", v, err)
	}
}
func TestScheduleAllowanceAndValidation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.db.Exec(ctx, "UPDATE servers SET backup_limit=1 WHERE id=$1", f.id); err != nil {
		t.Fatal(err)
	}
	in := Update{Enabled: true, Hour: 3, Timezone: "UTC"}
	if _, err := f.s.Put(ctx, f.owner, f.id, in, "test"); !errors.Is(err, ErrAllowance) {
		t.Fatalf("low allowance: %v", err)
	}
	in.Enabled = false
	in.Timezone = "Local"
	if _, err := f.s.Put(ctx, f.owner, f.id, in, "test"); !errors.Is(err, server.ErrInvalid) {
		t.Fatalf("bad timezone: %v", err)
	}
}

func TestScheduleCannotEnableWhenHostSchedulerDisabled(t *testing.T) {
	f := setup(t)
	f.s.Available = false
	ctx := context.Background()
	if _, err := f.s.Put(ctx, f.owner, f.id, Update{Enabled: true, Hour: 3, Timezone: "UTC"}, "test"); !errors.Is(err, control.ErrNotConfigured) {
		t.Fatalf("silent disabled host scheduler: %v", err)
	}
	v, err := f.s.Get(ctx, f.owner, f.id)
	if err != nil || v.Available {
		t.Fatalf("disabled availability: %+v %v", v, err)
	}
	if _, err = f.s.Put(ctx, f.owner, f.id, Update{Hour: 3, Timezone: "UTC"}, "test"); err != nil {
		t.Fatal("disable must remain available", err)
	}
}
