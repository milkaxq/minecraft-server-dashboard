package server

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/testutil"
)

func TestOfferingFiltersOptionsAndRejectsNewLegacyServers(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	svc := Service{DB: db}
	for _, kind := range []string{"VANILLA", "PAPER", "FABRIC", "SPIGOT", "PURPUR", "FORGE", "NEOFORGE"} {
		_, err := db.Exec(ctx, `INSERT INTO software_templates(server_type,egg_id,docker_image,startup,version_variable) VALUES($1,1,'java','minecraft','MINECRAFT_VERSION');`, kind)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Exec(ctx, "INSERT INTO minecraft_versions(server_type,version) VALUES($1,'1.21.1')", kind)
		if err != nil {
			t.Fatal(err)
		}
	}
	options, err := svc.SoftwareOptions(ctx)
	if err != nil || len(options) != 3 {
		t.Fatal(options, err)
	}
	for _, option := range options {
		if !NewSoftware(option.ServerType) {
			t.Fatal("legacy option advertised", option)
		}
	}
	for _, kind := range []string{"SPIGOT", "PURPUR", "VANILLA", "NEOFORGE"} {
		_, err = svc.CreateTx(ctx, nil, uuid.New(), CreateInput{PlanID: uuid.New(), Name: "World", MinecraftVersion: "1.21.1", ServerType: kind}, "test")
		if !errors.Is(err, ErrInvalid) {
			t.Fatal("new legacy server accepted", kind, err)
		}
	}
	owner, id := uuid.New(), uuid.New()
	_, err = db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name) VALUES($1,'existing@example.com','test','Player')", owner)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO servers(id,owner_id,plan_id,name,minecraft_version,server_type,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit) VALUES($1,$2,'10000000-0000-4000-8000-000000000001','Existing Forge','1.21.1','FORGE',2048,10240,100,2,0)`, id, owner)
	if err != nil {
		t.Fatal(err)
	}
	old, err := svc.Get(ctx, owner, id)
	if err != nil || old.ServerType != "FORGE" {
		t.Fatal("existing server lost", old, err)
	}
}
