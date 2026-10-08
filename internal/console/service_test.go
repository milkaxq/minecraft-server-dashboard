package console_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/console"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/httpapi"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
	"minecraft-hosting/internal/testutil"
)

type source struct {
	socket string
	calls  atomic.Int32
}

func (s *source) WebsocketCredentials(context.Context, string) (pterodactyl.WebsocketCredentials, error) {
	s.calls.Add(1)
	return pterodactyl.WebsocketCredentials{Socket: s.socket, Token: "privileged-wings-jwt"}, nil
}
func digest(raw string) []byte { d := sha256.Sum256([]byte(raw)); return d[:] }
func TestTicketAndLiveConsoleIsolation(t *testing.T) {
	db := testutil.Database(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	owner, other, node, id := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	token := strings.Repeat("a", 43)
	otherToken := strings.Repeat("b", 43)
	for _, u := range []uuid.UUID{owner, other} {
		if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at) VALUES($1,$2,'unused','Player',now())", u, u.String()+"@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range []struct {
		u     uuid.UUID
		token string
	}{{owner, token}, {other, otherToken}} {
		if _, err := db.Exec(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", digest(v.token), v.u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, "INSERT INTO nodes(id,pterodactyl_node_id,name,cpu_capacity,ram_capacity_mb,disk_capacity_mb) VALUES($1,1,'Test',200,4096,20480)", node); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO servers(id,owner_id,plan_id,node_id,pterodactyl_server_id,pterodactyl_identifier,name,status,minecraft_version,server_type,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit) VALUES($1,$2,'10000000-0000-4000-8000-000000000001',$3,1,'abcd1234','World','RUNNING','1.21.1','PAPER',2048,10240,100,2,0)`, id, owner, node); err != nil {
		t.Fatal(err)
	}
	var forwarded atomic.Int32
	wings := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "https://panel.example.com" {
			t.Error("wrong upstream origin")
		}
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"panel.example.com"}})
		if err != nil {
			t.Error(err)
			return
		}
		defer c.CloseNow()
		_, data, err := c.Read(r.Context())
		if err != nil {
			return
		}
		var authMessage struct {
			Event string
			Args  []string
		}
		if json.Unmarshal(data, &authMessage) != nil || authMessage.Event != "auth" || len(authMessage.Args) != 1 || authMessage.Args[0] != "privileged-wings-jwt" {
			t.Error("bad Wings auth")
			return
		}
		send := func(raw string) bool { return c.Write(r.Context(), websocket.MessageText, []byte(raw)) == nil }
		if !send(`{"event":"auth success"}`) {
			return
		}
		_, data, err = c.Read(r.Context())
		if err != nil {
			return
		}
		if string(data) != `{"event":"send logs"}` {
			t.Errorf("expected log request, got %s", data)
			return
		}
		_ = send(`{"event":"install output","args":["HOST_PRIVATE_INSTALL_LOG"]}`)
		_ = send(`{"event":"daemon error","args":["privileged-wings-jwt"]}`)
		_ = send(`{"event":"console output","args":["[Server] Done! Type help for commands."]}`)
		_ = send(`{"event":"status","args":["running"]}`)
		for {
			_, data, err = c.Read(r.Context())
			if err != nil {
				return
			}
			if strings.Contains(string(data), "set state") || strings.Contains(string(data), "send command") {
				forwarded.Add(1)
			}
		}
	}))
	defer wings.Close()
	endpoint := "ws" + strings.TrimPrefix(wings.URL, "http") + "/api/servers/" + uuid.NewString() + "/ws"
	upstream := &source{socket: endpoint}
	controls := &control.Service{DB: db}
	bridge := &console.Service{DB: db, Control: controls, Source: upstream, FrontendOrigin: "http://localhost:3000", PanelOrigin: "https://panel.example.com", AllowedOrigins: []string{"ws" + strings.TrimPrefix(wings.URL, "http")}, AllowPlaintext: true, RecheckInterval: 30 * time.Millisecond}
	mailer := unusedMail{}
	authentication, err := auth.New(db, mailer)
	if err != nil {
		t.Fatal(err)
	}
	api := &httpapi.API{DB: db, Auth: authentication, Servers: server.Service{DB: db}, Control: controls, Console: bridge, Origin: bridge.FrontendOrigin, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	proxy := httptest.NewServer(api.Handler())
	defer proxy.Close()
	if _, err = bridge.Mint(ctx, other, id, otherToken); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("foreign console ticket minted", err)
	}
	ticket, err := bridge.Mint(ctx, owner, id, token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bridge.Consume(ctx, uuid.New(), ticket.Token); !errors.Is(err, console.ErrTicket) {
		t.Fatal("ticket retargeted")
	}
	socketURL := "ws" + strings.TrimPrefix(proxy.URL, "http") + "/servers/" + id.String() + "/console"
	options := func(ticket string, origin string) *websocket.DialOptions {
		return &websocket.DialOptions{Subprotocols: []string{console.Protocol, "ticket." + ticket}, HTTPHeader: http.Header{"Origin": []string{origin}}}
	}
	if c, response, err := websocket.Dial(ctx, socketURL, options(ticket.Token, "https://evil.example.com")); err == nil {
		c.CloseNow()
		t.Fatal("foreign origin accepted")
	} else if response == nil || response.StatusCode != 403 {
		t.Fatal("wrong origin response", err)
	}
	browser, response, err := websocket.Dial(ctx, socketURL, options(ticket.Token, bridge.FrontendOrigin))
	if err != nil {
		t.Fatal("console upgrade failed", err, response)
	}
	defer browser.CloseNow()
	for _, expected := range []string{"connected", "console output", "status"} {
		_, data, err := browser.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			Event string
			Args  []string
		}
		if json.Unmarshal(data, &m) != nil || m.Event != expected {
			t.Fatalf("unexpected event %s", data)
		}
		if strings.Contains(string(data), "HOST_PRIVATE") || strings.Contains(string(data), "privileged-wings-jwt") {
			t.Fatal("upstream secrets exposed")
		}
	}
	if _, response, err = websocket.Dial(ctx, socketURL, options(ticket.Token, bridge.FrontendOrigin)); err == nil || response == nil || response.StatusCode != 401 {
		t.Fatal("consumed ticket replayed", err)
	}
	// Browser messages must never forward a privileged Wings operation.
	if err = browser.Write(ctx, websocket.MessageText, []byte(`{"event":"set state","args":["kill"]}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err = browser.Read(ctx); err == nil {
		t.Fatal("browser control injection not disconnected")
	}
	if forwarded.Load() != 0 {
		t.Fatal("Wings privileged event forwarded")
	}
	ticket, err = bridge.Mint(ctx, owner, id, token)
	if err != nil {
		t.Fatal(err)
	}
	browser2, _, err := websocket.Dial(ctx, socketURL, options(ticket.Token, bridge.FrontendOrigin))
	if err != nil {
		t.Fatal(err)
	}
	defer browser2.CloseNow()
	if _, _, err = browser2.Read(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "DELETE FROM sessions WHERE token_hash=$1", digest(token)); err != nil {
		t.Fatal(err)
	}
	revokeCtx, revokeCancel := context.WithTimeout(ctx, 2*time.Second)
	defer revokeCancel()
	closed := false
	for i := 0; i < 10; i++ {
		if _, _, err = browser2.Read(revokeCtx); err != nil {
			if revokeCtx.Err() != nil {
				t.Fatal("console did not close promptly after logout", err)
			}
			closed = true
			break
		}
	}
	if !closed {
		t.Fatal("logged out console remained open")
	}
	if _, err = bridge.Mint(ctx, owner, id, token); !errors.Is(err, console.ErrTicket) {
		t.Fatal("revoked session minted console ticket", err)
	}
}

type unusedMail struct{}

func (unusedMail) SendVerification(context.Context, string, string) error { return nil }
func TestSocketEgressValidation(t *testing.T) {
	s := &console.Service{AllowedOrigins: []string{"wss://wings.example.com:8080"}}
	id := uuid.NewString()
	good := "wss://wings.example.com:8080/api/servers/" + id + "/ws"
	if err := s.ValidateSocket(good); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"ws://wings.example.com:8080/api/servers/" + id + "/ws", "wss://evil.example.com/api/servers/" + id + "/ws", good + "?token=secret", "wss://user:secret@wings.example.com:8080/api/servers/" + id + "/ws", "wss://wings.example.com:8080/admin", strings.Replace(good, id, "not-a-uuid", 1)} {
		if err := s.ValidateSocket(bad); err == nil {
			t.Errorf("unsafe socket allowed %s", bad)
		}
	}
}
