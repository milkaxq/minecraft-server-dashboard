package management

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

func modJar(t *testing.T, descriptor, body string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, err := z.Create(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestModLoaderCompatibility(t *testing.T) {
	for _, test := range []struct {
		kind, file, body string
		valid            bool
	}{
		{"FABRIC", "fabric.mod.json", `{"id":"example","environment":"*"}`, true},
		{"FABRIC", "fabric.mod.json", `{"id":"example","environment":"client"}`, false},
		{"FABRIC", "fabric.mod.json", `not json`, false},
		{"FORGE", "META-INF/mods.toml", `modLoader="javafml"`, true},
		{"NEOFORGE", "META-INF/neoforge.mods.toml", `modLoader="javafml"`, true},
		{"FORGE", "META-INF/neoforge.mods.toml", `modLoader="javafml"`, false},
		{"NEOFORGE", "META-INF/mods.toml", `modLoader="javafml"`, false},
		{"FABRIC", "plugin.yml", `name: example`, false},
		{"PAPER", "fabric.mod.json", `{"id":"example"}`, false},
	} {
		if got := validateMod(test.kind, "example.jar", modJar(t, test.file, test.body)); (got == nil) != test.valid {
			t.Errorf("%s/%s: %v", test.kind, test.file, got)
		}
	}
	if validateMod("FABRIC", "large.jar", modJar(t, "fabric.mod.json", strings.Repeat(" ", 65<<10))) == nil {
		t.Fatal("oversized compressed metadata")
	}
	if validateMod("FABRIC", "../bad.jar", modJar(t, "fabric.mod.json", `{"id":"example"}`)) == nil {
		t.Fatal("unsafe name")
	}
	if validatePluginForSoftware("SPIGOT", "example.jar", pluginJar(t, "paper-plugin.yml")) == nil {
		t.Fatal("Paper-only plugin accepted by Spigot")
	}
}
func TestModManagementAuthorizationAndReplay(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.db.Exec(ctx, "UPDATE servers SET server_type='FABRIC' WHERE id=$1", f.id); err != nil {
		t.Fatal(err)
	}
	data := modJar(t, "fabric.mod.json", `{"id":"example","environment":"*"}`)
	if _, err := f.s.UploadMod(ctx, f.other, f.id, uuid.New(), "example.jar", data, "test"); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("outsider", err)
	}
	if _, err := f.s.UploadMod(ctx, f.mod, f.id, uuid.New(), "example.jar", data, "test"); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("moderator", err)
	}
	if _, err := f.s.UploadMod(ctx, f.owner, f.id, uuid.New(), "wrong.jar", modJar(t, "plugin.yml", "name: Wrong"), "test"); !errors.Is(err, ErrMod) {
		t.Fatal("incompatible mod is not a definite rejection", err)
	}
	var unresolved int
	if err := f.db.QueryRow(ctx, "SELECT count(*) FROM management_operations WHERE server_id=$1 AND status IN ('PENDING','UNKNOWN')", f.id).Scan(&unresolved); err != nil || unresolved != 0 {
		t.Fatal("invalid mod blocked further operations", unresolved, err)
	}
	key := uuid.New()
	if _, err := f.s.UploadMod(ctx, f.owner, f.id, key, "example.jar", data, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.UploadMod(ctx, f.owner, f.id, key, "example.jar", data, "test"); err != nil {
		t.Fatal("replay", err)
	}
	list, err := f.s.Mods(ctx, f.owner, f.id)
	if err != nil || len(list) != 1 {
		t.Fatal("list", list, err)
	}
	if _, err = f.s.DeleteMod(ctx, f.owner, f.id, uuid.New(), "example.jar", "", "test"); !errors.Is(err, server.ErrInvalid) {
		t.Fatal("confirmation", err)
	}
	if err = f.panel.Power(ctx, f.identifier, pterodactyl.PowerStart); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DeleteMod(ctx, f.owner, f.id, uuid.New(), "example.jar", "example.jar", "test"); !errors.Is(err, ErrOffline) {
		t.Fatal("online mutation", err)
	}
	if err = f.panel.Power(ctx, f.identifier, pterodactyl.PowerStop); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DeleteMod(ctx, f.owner, f.id, uuid.New(), "example.jar", "example.jar", "test"); err != nil {
		t.Fatal(err)
	}
}
