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

type moderationPanel struct {
	*memoryPanel
	commands []string
	ignore   bool
}

func (p *moderationPanel) SendCommand(ctx context.Context, id, command string) error {
	if err := p.Mock.SendCommand(ctx, id, command); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands = append(p.commands, command)
	if p.ignore {
		return nil
	}
	parts := strings.SplitN(command, " ", 3)
	var players []BannedPlayer
	_ = json.Unmarshal(p.files["/banned-players.json"], &players)
	if parts[0] == "minecraft:ban" {
		players = append(players, BannedPlayer{UUID: uuid.NewString(), Name: parts[1], Reason: parts[2]})
	}
	if parts[0] == "minecraft:pardon" {
		out := []BannedPlayer{}
		for _, v := range players {
			if !strings.EqualFold(v.Name, parts[1]) {
				out = append(out, v)
			}
		}
		players = out
	}
	data, _ := json.Marshal(players)
	p.files["/banned-players.json"] = data
	return nil
}
func TestPlayerActionsRejectUnsafeOrUnconfirmedCommands(t *testing.T) {
	for _, in := range []PlayerAction{
		{Action: "ban", Name: "Steve\nstop", Reason: "Rules", Confirmed: true},
		{Action: "ban", Name: "@a", Reason: "Rules", Confirmed: true},
		{Action: "kick", Name: "Steve", Reason: "Rules\nop Steve", Confirmed: true},
		{Action: "kick", Name: "Steve", Reason: "Rules\u2028stop", Confirmed: true},
		{Action: "ban", Name: "Steve", Reason: strings.Repeat("x", 161), Confirmed: true},
		{Action: "ban", Name: "Steve", Reason: "Rules"},
		{Action: "op", Name: "Steve", Reason: "Rules", Confirmed: true},
		{Action: "unban", Name: "Steve", Reason: "Rules", Confirmed: true},
	} {
		in.Revision = strings.Repeat("a", 64)
		if _, err := in.command(); !errors.Is(err, server.ErrInvalid) {
			t.Fatal("unsafe input accepted", in, err)
		}
	}
}
func TestPlayerModerationAuthorizationRevisionReplayAndConfirmation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	p := &moderationPanel{memoryPanel: f.panel}
	f.s.Control.Panel = p
	for _, user := range []uuid.UUID{f.other, f.mod} {
		before := f.panel.calls
		if _, err := f.s.PlayerBans(ctx, user, f.id); !errors.Is(err, server.ErrNotFound) {
			t.Fatal(err)
		}
		if _, err := f.s.ChangePlayer(ctx, user, f.id, uuid.New(), PlayerAction{Action: "ban", Name: "Steve", Reason: "Rules", Confirmed: true, Revision: strings.Repeat("a", 64)}, "test"); !errors.Is(err, server.ErrNotFound) {
			t.Fatal(err)
		}
		if f.panel.calls != before {
			t.Fatal("unauthorized Panel access")
		}
	}
	if err := p.Power(ctx, f.identifier, pterodactyl.PowerStart); err != nil {
		t.Fatal(err)
	}
	list, err := f.s.PlayerBans(ctx, f.owner, f.id)
	if err != nil {
		t.Fatal(err)
	}
	in := PlayerAction{Action: "ban", Name: "Steve", Reason: "Be kind", Confirmed: true, Revision: strings.Repeat("a", 64)}
	if _, err = f.s.ChangePlayer(ctx, f.owner, f.id, uuid.New(), in, "test"); !errors.Is(err, ErrChanged) {
		t.Fatal("stale edit accepted", err)
	}
	in.Revision = list.Revision
	key := uuid.New()
	for i := 0; i < 2; i++ {
		result, err := f.s.ChangePlayer(ctx, f.owner, f.id, key, in, "test")
		if err != nil || result.(map[string]any)["confirmed"] != true {
			t.Fatal(result, err)
		}
	}
	if len(p.commands) != 1 {
		t.Fatal("command replayed", p.commands)
	}
	list, err = f.s.PlayerBans(ctx, f.admin, f.id)
	if err != nil || len(list.Players) != 1 || list.Players[0].Reason != "Be kind" {
		t.Fatal(list, err)
	}
	result, err := f.s.ChangePlayer(ctx, f.admin, f.id, uuid.New(), PlayerAction{Action: "unban", Name: "Steve", Confirmed: true, Revision: list.Revision}, "test")
	if err != nil || result.(map[string]any)["confirmed"] != true {
		t.Fatal(result, err)
	}
	list, err = f.s.PlayerBans(ctx, f.owner, f.id)
	if err != nil || len(list.Players) != 0 {
		t.Fatal(list, err)
	}
	result, err = f.s.ChangePlayer(ctx, f.owner, f.id, uuid.New(), PlayerAction{Action: "kick", Name: "Steve", Reason: "Take a break", Confirmed: true, Revision: list.Revision}, "test")
	if err != nil || result.(map[string]any)["confirmed"] != false || result.(map[string]any)["sent"] != true {
		t.Fatal("kick confirmation invented", result, err)
	}
	var count int
	if err = f.db.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE server_id=$1 AND action='management.completed' AND metadata->>'action' LIKE 'player.%' AND metadata->>'outcome'='SUCCEEDED'", f.id).Scan(&count); err != nil || count != 3 {
		t.Fatal("audit missing", count, err)
	}
}
func TestPlayerBansRejectSymlinksAndMalformedFiles(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.panel.files["/banned-players.json"] = []byte(`[{"name":"Steve","uuid":"invalid"}]`)
	if _, err := f.s.PlayerBans(ctx, f.owner, f.id); !errors.Is(err, server.ErrInvalid) {
		t.Fatal(err)
	}
	f.panel.files["/banned-players.json"] = []byte("[]")
	f.panel.symlinks["/banned-players.json"] = true
	if _, err := f.s.PlayerBans(ctx, f.owner, f.id); !errors.Is(err, server.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestPlayerBanDoesNotTreatDeliveryAsExecution(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	p := &moderationPanel{memoryPanel: f.panel, ignore: true}
	f.s.Control.Panel = p
	list, err := f.s.PlayerBans(ctx, f.owner, f.id)
	if err != nil {
		t.Fatal(err)
	}
	in := PlayerAction{Action: "ban", Name: "Steve", Reason: "Rules", Confirmed: true, Revision: list.Revision}
	if _, err = f.s.ChangePlayer(ctx, f.owner, f.id, uuid.New(), in, "test"); err == nil {
		t.Fatal("offline ban accepted")
	}
	if err = p.Power(ctx, f.identifier, pterodactyl.PowerStart); err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	for i := 0; i < 2; i++ {
		result, err := f.s.ChangePlayer(ctx, f.owner, f.id, key, in, "test")
		if err != nil || result.(map[string]any)["confirmed"] != false {
			t.Fatal("invented ban confirmation", result, err)
		}
	}
	if len(p.commands) != 1 {
		t.Fatal("unconfirmed command replayed", p.commands)
	}
}
