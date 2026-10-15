package management

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"minecraft-hosting/internal/server"
)

func TestChunkyWorldsPermissionsAndExistingCustomWorlds(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.panel.files["/server.properties"] = []byte("level-name=old\nlevel-name=Campfire\nrcon.password=never-return\n")
	f.panel.folders["/Campfire"] = true
	f.panel.folders["/Campfire_nether"] = true
	f.panel.folders["/Campfire_the_end"] = true
	f.panel.symlinks["/Campfire_the_end"] = true
	f.panel.folders["/plugins"] = true
	f.panel.folders["/other-world"] = true
	before := f.panel.calls
	if _, err := f.s.ChunkyWorlds(ctx, f.other, f.id); !errors.Is(err, server.ErrNotFound) {
		t.Fatal(err)
	}
	if f.panel.calls != before {
		t.Fatal("foreign caller contacted Panel")
	}
	for _, user := range []uuid.UUID{f.owner, f.admin, f.mod} {
		worlds, err := f.s.ChunkyWorlds(ctx, user, f.id)
		if err != nil {
			t.Fatal(err)
		}
		if len(worlds) != 2 || worlds[0].Name != "Campfire" || worlds[0].Label != "Main world" || worlds[1].Name != "Campfire_nether" {
			t.Fatal(worlds)
		}
	}
	// A folder name must be a single command argument, not a path or injected command.
	for _, name := range []string{"../host", "world name", "world;stop", "world\\nstop", strings.Repeat("a", 65)} {
		f.panel.files["/server.properties"] = []byte("level-name=" + name + "\n")
		if _, err := f.s.ChunkyWorlds(ctx, f.owner, f.id); !errors.Is(err, ErrAddonConfig) {
			t.Fatalf("accepted %q: %v", name, err)
		}
	}
}

func TestChunkyWorldsAvoidInventingDimensionsAndRejectSymlinkProperties(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	worlds, err := f.s.ChunkyWorlds(ctx, f.owner, f.id)
	if err != nil || len(worlds) != 0 {
		t.Fatal(worlds, err)
	}
	f.panel.folders["/world"] = true
	f.panel.folders["/world_nether"] = true
	if _, err = f.db.Exec(ctx, "UPDATE servers SET server_type='FABRIC' WHERE id=$1", f.id); err != nil {
		t.Fatal(err)
	}
	worlds, err = f.s.ChunkyWorlds(ctx, f.owner, f.id)
	if err != nil || len(worlds) != 1 || worlds[0].Name != "world" {
		t.Fatal(worlds, err)
	}
	f.panel.symlinks["/server.properties"] = true
	if _, err = f.s.ChunkyWorlds(ctx, f.owner, f.id); !errors.Is(err, server.ErrInvalid) {
		t.Fatal("followed symlink", err)
	}
}
