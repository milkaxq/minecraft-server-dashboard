package node

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/testutil"
)

type discovery struct {
	info        pterodactyl.NodeInfo
	allocations []pterodactyl.Allocation
}

func (d discovery) GetNode(context.Context, int) (pterodactyl.NodeInfo, error) { return d.info, nil }
func (d discovery) ListAllocations(context.Context, int) ([]pterodactyl.Allocation, error) {
	return d.allocations, nil
}
func TestRegisterDedicatedNode(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	admin, customer := uuid.New(), uuid.New()
	if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,is_admin,verified_at) VALUES($1,'admin@example.com','unused','Admin',true,now()),($2,'customer@example.com','unused','Player',false,now())", admin, customer); err != nil {
		t.Fatal(err)
	}
	d := discovery{info: pterodactyl.NodeInfo{ID: 1, Name: "Dedicated", RAMMB: 8192, DiskMB: 40960}, allocations: []pterodactyl.Allocation{{ID: 51, Hostname: "play.example.com", Port: 25565}}}
	if _, err := Register(ctx, db, d, customer, 1, 4096, 20480, 200, true); err == nil {
		t.Fatal("customer registered infrastructure")
	}
	if _, err := Register(ctx, db, d, admin, 1, 16384, 20480, 200, true); err == nil {
		t.Fatal("physical capacity exceeded")
	}
	if _, err := Register(ctx, db, d, admin, 1, 4096, 20480, 200, false); err == nil {
		t.Fatal("shared node accepted")
	}
	occupied := d
	occupied.info.AllocatedRAMMB = 1
	if _, err := Register(ctx, db, occupied, admin, 1, 4096, 20480, 200, true); err == nil {
		t.Fatal("nonempty node accepted")
	}
	assigned := d
	assigned.allocations = []pterodactyl.Allocation{{ID: 51, Hostname: "play.example.com", Port: 25565, Assigned: true}}
	if _, err := Register(ctx, db, assigned, admin, 1, 4096, 20480, 200, true); err == nil {
		t.Fatal("externally assigned allocation accepted")
	}
	id, err := Register(ctx, db, d, admin, 1, 4096, 20480, 200, true)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM node_allocations WHERE node_id=$1", id).Scan(&count); err != nil || count != 1 {
		t.Fatal("allocations not imported", err)
	}
	if err = db.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE action='node.registered' AND actor_id=$1 AND server_id IS NULL", admin).Scan(&count); err != nil || count != 1 {
		t.Fatal("node registration not audited", err)
	}
}
