// Package console relays only Minecraft console output, never host/admin events or
// client-supplied Wings messages. Commands travel through authorized REST handlers.
package console

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

const Protocol = "hosting.console.v1"

var ErrTicket = errors.New("invalid or expired console ticket")

type Ticket struct {
	Token     string    `json:"ticket"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Access struct {
	UserID, ServerID uuid.UUID
	SessionHash      []byte
}
type Service struct {
	DB                          *pgxpool.Pool
	Control                     *control.Service
	Source                      pterodactyl.ConsoleClient
	FrontendOrigin, PanelOrigin string
	AllowedOrigins              []string
	AllowPlaintext              bool
	Lifetime                    context.Context
	RecheckInterval             time.Duration
	mu                          sync.Mutex
	connections                 map[uuid.UUID]int
	total                       int
}

func (s *Service) Configured() bool { return s != nil && s.Source != nil && len(s.AllowedOrigins) > 0 }
func hash(token string) []byte      { v := sha256.Sum256([]byte(token)); return v[:] }
func (s *Service) Mint(ctx context.Context, user, id uuid.UUID, session string) (Ticket, error) {
	if _, err := s.Control.Check(ctx, user, id, server.Console); err != nil {
		return Ticket{}, err
	}
	if !s.Configured() {
		return Ticket{}, control.ErrNotConfigured
	}
	if len(session) != 43 {
		return Ticket{}, ErrTicket
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Ticket{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expiry := time.Now().UTC().Add(45 * time.Second)
	tag, err := s.DB.Exec(ctx, `INSERT INTO console_tickets(token_hash,server_id,user_id,session_hash,expires_at)
 SELECT $1,$2,$3,token_hash,$4 FROM sessions WHERE token_hash=$5 AND user_id=$3 AND expires_at>now()`, hash(token), id, user, expiry, hash(session))
	if err != nil {
		return Ticket{}, err
	}
	if tag.RowsAffected() != 1 {
		return Ticket{}, ErrTicket
	}
	return Ticket{Token: token, ExpiresAt: expiry}, nil
}
func (s *Service) Consume(ctx context.Context, id uuid.UUID, token string) (Access, error) {
	if len(token) != 43 {
		return Access{}, ErrTicket
	}
	var a Access
	err := s.DB.QueryRow(ctx, `DELETE FROM console_tickets WHERE token_hash=$1 AND server_id=$2 AND expires_at>now() RETURNING user_id,server_id,session_hash`, hash(token), id).Scan(&a.UserID, &a.ServerID, &a.SessionHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrTicket
	}
	if err != nil {
		return a, err
	}
	if _, err = s.Revalidate(ctx, a); err != nil {
		return a, err
	}
	return a, nil
}
func (s *Service) Revalidate(ctx context.Context, a Access) (control.Target, error) {
	var valid bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sessions se JOIN users u ON u.id=se.user_id WHERE se.token_hash=$1 AND se.user_id=$2 AND se.expires_at>now() AND u.verified_at IS NOT NULL)`, a.SessionHash, a.UserID).Scan(&valid); err != nil {
		return control.Target{}, err
	}
	if !valid {
		return control.Target{}, ErrTicket
	}
	return s.Control.Check(ctx, a.UserID, a.ServerID, server.Console)
}
func (s *Service) acquire(id uuid.UUID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.connections == nil {
		s.connections = map[uuid.UUID]int{}
	}
	if s.total >= 128 || s.connections[id] >= 2 {
		return false
	}
	s.total++
	s.connections[id]++
	return true
}
func (s *Service) release(id uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total--
	s.connections[id]--
	if s.connections[id] == 0 {
		delete(s.connections, id)
	}
}
func (s *Service) ValidateSocket(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "wss" && !(s.AllowPlaintext && u.Scheme == "ws")) {
		return control.ErrUnavailable
	}
	allowed := false
	for _, origin := range s.AllowedOrigins {
		if u.Scheme+"://"+u.Host == origin {
			allowed = true
		}
	}
	if !allowed {
		return control.ErrUnavailable
	}
	parts := strings.Split(u.Path, "/")
	if len(parts) != 5 || parts[1] != "api" || parts[2] != "servers" || parts[4] != "ws" {
		return control.ErrUnavailable
	}
	if _, err = uuid.Parse(parts[3]); err != nil {
		return control.ErrUnavailable
	}
	return nil
}

type message struct {
	Event string   `json:"event"`
	Args  []string `json:"args,omitempty"`
}

func write(ctx context.Context, c *websocket.Conn, m message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return c.Write(deadline, websocket.MessageText, data)
}

// Relay validates upstream before upgrading. After upgrade all failures close the
// stream with generic messages, without serializing privileged upstream errors.
func (s *Service) Relay(w http.ResponseWriter, r *http.Request, a Access) error {
	if !s.Configured() {
		return control.ErrNotConfigured
	}
	if !s.acquire(a.UserID) {
		return control.ErrBusy
	}
	defer s.release(a.UserID)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	if s.Lifetime != nil {
		stop := context.AfterFunc(s.Lifetime, cancel)
		defer stop()
	}
	target, err := s.Revalidate(ctx, a)
	if err != nil {
		return err
	}
	credentials, err := s.Source.WebsocketCredentials(ctx, target.Identifier)
	if err != nil {
		return control.ErrUnavailable
	}
	if err = s.ValidateSocket(credentials.Socket); err != nil {
		return err
	}
	connect, cancelConnect := context.WithTimeout(ctx, 10*time.Second)
	upstream, response, err := websocket.Dial(connect, credentials.Socket, &websocket.DialOptions{HTTPClient: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, HTTPHeader: http.Header{"Origin": []string{s.PanelOrigin}}})
	cancelConnect()
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return control.ErrUnavailable
	}
	defer upstream.CloseNow()
	upstream.SetReadLimit(64 << 10)
	if err = write(ctx, upstream, message{Event: "auth", Args: []string{credentials.Token}}); err != nil {
		return control.ErrUnavailable
	}
	// Wait for Wings authentication before declaring the browser connected.
	authCtx, authCancel := context.WithTimeout(ctx, 10*time.Second)
	_, data, err := upstream.Read(authCtx)
	authCancel()
	var first message
	if err != nil || json.Unmarshal(data, &first) != nil || first.Event != "auth success" {
		return control.ErrUnavailable
	}
	if _, err = s.Revalidate(ctx, a); err != nil {
		return err
	}
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Time{})
	_ = controller.SetWriteDeadline(time.Time{})
	origin, _ := url.Parse(s.FrontendOrigin)
	browser, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{Protocol}, OriginPatterns: []string{origin.Host}})
	if err != nil {
		return nil
	}
	defer browser.CloseNow()
	browser.SetReadLimit(4096)
	// There is deliberately no generic browser→Wings forwarding path.
	go func() {
		defer cancel()
		_, _, err := browser.Read(ctx)
		if err == nil {
			_ = browser.Close(websocket.StatusPolicyViolation, "Console input must use the authorized command API")
		}
	}()
	interval := s.RecheckInterval
	if interval <= 0 {
		interval = 15 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				check, done := context.WithTimeout(ctx, 5*time.Second)
				_, err := s.Revalidate(check, a)
				if err == nil {
					err = browser.Ping(check)
				}
				done()
				if err != nil {
					return
				}
			}
		}
	}()
	if err = write(ctx, browser, message{Event: "connected"}); err != nil {
		return nil
	}
	if err = write(ctx, upstream, message{Event: "send logs"}); err != nil {
		return nil
	}
	for {
		kind, data, err := upstream.Read(ctx)
		if err != nil {
			break
		}
		if kind != websocket.MessageText {
			continue
		}
		var m message
		if json.Unmarshal(data, &m) != nil {
			break
		}
		switch m.Event {
		case "console output":
			if len(m.Args) != 1 {
				continue
			}
			line := m.Args[0]
			if len(line) > 8192 {
				line = line[:8192] + " …"
			}
			if err = write(ctx, browser, message{Event: "console output", Args: []string{line}}); err != nil {
				return nil
			}
		case "status":
			if len(m.Args) == 1 && (m.Args[0] == "running" || m.Args[0] == "offline" || m.Args[0] == "starting" || m.Args[0] == "stopping") {
				if err = write(ctx, browser, m); err != nil {
					return nil
				}
			}
		case "token expiring":
			if _, err = s.Revalidate(ctx, a); err != nil {
				return nil
			}
			renewed, err := s.Source.WebsocketCredentials(ctx, target.Identifier)
			if err != nil || renewed.Socket != credentials.Socket || s.ValidateSocket(renewed.Socket) != nil {
				return nil
			}
			if err = write(ctx, upstream, message{Event: "auth", Args: []string{renewed.Token}}); err != nil {
				return nil
			}
		case "jwt error", "token expired":
			return nil
			// Stats, installation/transfer logs, administrative errors and tokens are not exposed.
		}
	}
	_ = browser.Close(websocket.StatusGoingAway, "Console disconnected; reconnect to resume")
	return nil
}
