package management

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

type whitelistPanel struct {
	*memoryPanel
	commands []string
	ignore   bool
}

func (p *whitelistPanel) SendCommand(ctx context.Context, id, command string) error {
	if err := p.Mock.SendCommand(ctx, id, command); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands = append(p.commands, command)
	if p.ignore {
		return nil
	}
	parts := strings.Fields(command)
	var players []WhitelistedPlayer
	_ = json.Unmarshal(p.files["/whitelist.json"], &players)
	switch parts[1] {
	case "add":
		players = append(players, WhitelistedPlayer{uuid.NewString(), parts[2]})
	case "remove":
		out := []WhitelistedPlayer{}
		for _, player := range players {
			if !strings.EqualFold(player.Name, parts[2]) {
				out = append(out, player)
			}
		}
		players = out
	case "on", "off":
		p.files["/server.properties"] = []byte("white-list=" + map[string]string{"on": "true", "off": "false"}[parts[1]] + "\n")
	}
	data, _ := json.Marshal(players)
	p.files["/whitelist.json"] = data
	return nil
}

func TestWhitelistRejectsCommandInjectionAndUnconfirmedChanges(t *testing.T) {
	for _, name := range []string{"", "ab", "name with space", "Steve\nstop", "@a", "../../world", "Name;op", "abcdefghijklmnopq", "éPlayer"} {
		if _, err := (WhitelistChange{Action: "add", Name: name, Revision: strings.Repeat("a", 64)}).command(); !errors.Is(err, server.ErrInvalid) {
			t.Fatalf("accepted %q: %v", name, err)
		}
	}
	for _, in := range []WhitelistChange{
		{Action: "remove", Name: "Steve", Revision: strings.Repeat("a", 64)},
		{Action: "off", Revision: strings.Repeat("a", 64)},
		{Action: "on", Name: "Steve", Revision: strings.Repeat("a", 64)},
		{Action: "op", Name: "Steve", Revision: strings.Repeat("a", 64)},
	} {
		if _, err := in.command(); !errors.Is(err, server.ErrInvalid) {
			t.Fatal("accepted unsafe change", in, err)
		}
	}
	command, err := (WhitelistChange{Action: "add", Name: "Player_123", Revision: strings.Repeat("a", 64)}).command()
	if err != nil || command != "minecraft:whitelist add Player_123" {
		t.Fatal(command, err)
	}
}

func TestWhitelistAuthorizationRevisionReplayAndObservedResult(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	p := &whitelistPanel{memoryPanel: f.panel}
	f.s.Control.Panel = p
	f.panel.files["/whitelist.json"] = []byte("[]")
	for _, user := range []uuid.UUID{f.other, f.mod} {
		before := f.panel.calls
		if _, err := f.s.Whitelist(ctx, user, f.id); !errors.Is(err, server.ErrNotFound) {
			t.Fatal("unauthorized read", err)
		}
		if _, err := f.s.ChangeWhitelist(ctx, user, f.id, uuid.New(), WhitelistChange{Action: "add", Name: "Steve", Revision: strings.Repeat("a", 64)}, "test"); !errors.Is(err, server.ErrNotFound) {
			t.Fatal("unauthorized write", err)
		}
		if f.panel.calls != before {
			t.Fatal("unauthorized request contacted Panel")
		}
	}
	list, err := f.s.Whitelist(ctx, f.admin, f.id)
	if err != nil || list.Enabled || len(list.Players) != 0 {
		t.Fatal(list, err)
	}
	in := WhitelistChange{Action: "add", Name: "Steve", Revision: list.Revision}
	if _, err = f.s.ChangeWhitelist(ctx, f.owner, f.id, uuid.New(), in, "test"); !errors.Is(err, control.ErrState) {
		t.Fatal("offline change accepted", err)
	}
	if err = p.Power(ctx, f.identifier, pterodactyl.PowerStart); err != nil {
		t.Fatal(err)
	}
	stale := in
	stale.Revision = strings.Repeat("b", 64)
	if _, err = f.s.ChangeWhitelist(ctx, f.owner, f.id, uuid.New(), stale, "test"); !errors.Is(err, ErrChanged) {
		t.Fatal("stale revision accepted", err)
	}
	key := uuid.New()
	for range 2 {
		result, err := f.s.ChangeWhitelist(ctx, f.owner, f.id, key, in, "test")
		if err != nil || result.(map[string]any)["confirmed"] != true {
			t.Fatal(result, err)
		}
	}
	if len(p.commands) != 1 || p.commands[0] != "minecraft:whitelist add Steve" {
		t.Fatal("duplicate or unsafe command", p.commands)
	}
	list, err = f.s.Whitelist(ctx, f.owner, f.id)
	if err != nil || len(list.Players) != 1 || list.Players[0].Name != "Steve" {
		t.Fatal(list, err)
	}
	for _, change := range []WhitelistChange{{Action: "on"}, {Action: "remove", Name: "Steve", Confirmed: true}, {Action: "off", Confirmed: true}} {
		change.Revision = list.Revision
		result, err := f.s.ChangeWhitelist(ctx, f.admin, f.id, uuid.New(), change, "test")
		if err != nil || result.(map[string]any)["confirmed"] != true {
			t.Fatal(result, err)
		}
		list, err = f.s.Whitelist(ctx, f.owner, f.id)
		if err != nil {
			t.Fatal(err)
		}
	}
	var audited int
	if err = f.db.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE server_id=$1 AND action='management.completed' AND metadata->>'action' LIKE 'whitelist.%' AND metadata->>'outcome'='SUCCEEDED'", f.id).Scan(&audited); err != nil || audited != 4 {
		t.Fatal("missing audit", audited, err)
	}
}

func TestWhitelistNeverTreatsCommandDeliveryAsSuccess(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	p := &whitelistPanel{memoryPanel: f.panel, ignore: true}
	f.s.Control.Panel = p
	if err := p.Power(ctx, f.identifier, pterodactyl.PowerStart); err != nil {
		t.Fatal(err)
	}
	list, err := f.s.Whitelist(ctx, f.owner, f.id)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.s.ChangeWhitelist(ctx, f.owner, f.id, uuid.New(), WhitelistChange{Action: "add", Name: "Steve", Revision: list.Revision}, "test")
	if err != nil || result.(map[string]any)["confirmed"] != false || len(p.commands) != 1 {
		t.Fatal(result, err, p.commands)
	}
}

func TestWhitelistRejectsSymlinkAndMalformedFile(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.panel.files["/whitelist.json"] = []byte(`[{"name":"Steve","uuid":"not-a-uuid"}]`)
	if _, err := f.s.Whitelist(ctx, f.owner, f.id); !errors.Is(err, server.ErrInvalid) {
		t.Fatal(err)
	}
	f.panel.files["/whitelist.json"] = []byte("[]")
	f.panel.symlinks["/whitelist.json"] = true
	if _, err := f.s.Whitelist(ctx, f.owner, f.id); !errors.Is(err, server.ErrInvalid) {
		t.Fatal("symlink followed", err)
	}
}
