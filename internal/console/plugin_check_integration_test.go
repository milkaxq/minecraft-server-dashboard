package console_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"minecraft-hosting/internal/console"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
	"minecraft-hosting/internal/testutil"
)

type pluginPanel struct {
	*pterodactyl.Mock
	commanded chan struct{}
	calls     atomic.Int32
}

func (p *pluginPanel) SendCommand(ctx context.Context, id, command string) error {
	if command != "bukkit:plugins" {
		return errors.New("unexpected command")
	}
	p.calls.Add(1)
	if err := p.Mock.SendCommand(ctx, id, command); err != nil {
		return err
	}
	p.commanded <- struct{}{}
	return nil
}
func TestPluginCheckAuthorizesAndObservesFreshRuntimeResponse(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	owner, other, mod, node, id := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, u := range []uuid.UUID{owner, other, mod} {
		if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at) VALUES($1,$2,'unused','Player',now())", u, u.String()+"@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	panel := &pluginPanel{Mock: pterodactyl.NewMock(), commanded: make(chan struct{}, 1)}
	remoteUser, err := panel.CreateUser(ctx, pterodactyl.UserRequest{ExternalID: owner, Email: "player@example.com", Name: "Player"})
	if err != nil {
		t.Fatal(err)
	}
	remote, err := panel.CreateServer(ctx, pterodactyl.ServerRequest{ExternalID: id, UserID: remoteUser.ID, NodeID: 1, AllocationID: 1, EggID: 1, Name: "World", Image: "java", Startup: "minecraft", Limits: pterodactyl.Limits{RAMMB: 2048, DiskMB: 10240, CPU: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "INSERT INTO nodes(id,pterodactyl_node_id,name,cpu_capacity,ram_capacity_mb,disk_capacity_mb) VALUES($1,1,'Test',200,4096,20480)", node); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO servers(id,owner_id,plan_id,node_id,pterodactyl_server_id,pterodactyl_identifier,name,status,minecraft_version,server_type,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit) VALUES($1,$2,'10000000-0000-4000-8000-000000000001',$3,$4,$5,'World','RUNNING','1.21.1','PAPER',2048,10240,100,0,0)`, id, owner, node, remote.ID, remote.Identifier); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "INSERT INTO server_members(server_id,user_id,role) VALUES($1,$2,'MODERATOR')", id, mod); err != nil {
		t.Fatal(err)
	}
	if err = panel.Power(ctx, remote.Identifier, pterodactyl.PowerStart); err != nil {
		t.Fatal(err)
	}
	wings := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		socket, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"panel.example.com"}})
		if err != nil {
			return
		}
		defer socket.CloseNow()
		_, raw, err := socket.Read(r.Context())
		if err != nil || !strings.Contains(string(raw), "privileged-wings-jwt") {
			return
		}
		_ = socket.Write(r.Context(), websocket.MessageText, []byte(`{"event":"auth success"}`))
		select {
		case <-panel.commanded:
		case <-r.Context().Done():
			return
		}
		// Host install output must not become runtime evidence or reach the response.
		for _, frame := range []map[string]any{{"event": "install output", "args": []string{"SECRET"}}, {"event": "console output", "args": []string{"Plugins (2): \x1b[32mVoiceChat\x1b[0m, \x1b[31mBrokenPlugin\x1b[0m"}}} {
			data, _ := json.Marshal(frame)
			_ = socket.Write(r.Context(), websocket.MessageText, data)
		}
		_, _, _ = socket.Read(r.Context())
	}))
	defer wings.Close()
	upstream := &source{socket: "ws" + strings.TrimPrefix(wings.URL, "http") + "/api/servers/" + uuid.NewString() + "/ws"}
	svc := &console.Service{DB: db, Control: &control.Service{DB: db, Panel: panel}, Source: upstream, PanelOrigin: "https://panel.example.com", AllowedOrigins: []string{"ws" + strings.TrimPrefix(wings.URL, "http")}, AllowPlaintext: true}
	for _, u := range []uuid.UUID{other, mod} {
		if _, err = svc.CheckPlugins(ctx, u, id, uuid.New(), "test"); !errors.Is(err, server.ErrNotFound) {
			t.Fatal("unauthorized plugin check", err)
		}
	}
	if upstream.calls.Load() != 0 || panel.calls.Load() != 0 {
		t.Fatal("unauthorized check reached Wings")
	}
	result, err := svc.CheckPlugins(ctx, owner, id, uuid.New(), "test")
	if err != nil || !result.Confirmed || len(result.Plugins) != 2 || result.Plugins[0].Enabled || !result.Plugins[1].Enabled {
		t.Fatal(result, err)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "SECRET") || strings.Contains(string(encoded), "privileged") {
		t.Fatal("upstream data leaked")
	}
	if panel.calls.Load() != 1 {
		t.Fatal("check repeated command")
	}
	if err = panel.Power(ctx, remote.Identifier, pterodactyl.PowerStop); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.CheckPlugins(ctx, owner, id, uuid.New(), "test"); !errors.Is(err, control.ErrState) {
		t.Fatal("offline check accepted", err)
	}
}
