package httpapi

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"minecraft-hosting/internal/console"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

func (a *API) liveLimit(bucket string, limit int, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.Auth.RateLimit(r.Context(), "live:"+bucket+":"+currentUser(r).ID.String(), limit); err != nil {
			a.fail(w, r, err)
			return
		}
		next(w, r)
	}
}
func (a *API) usage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		a.fail(w, r, server.ErrNotFound)
		return
	}
	if a.Control == nil {
		a.fail(w, r, control.ErrNotConfigured)
		return
	}
	u, err := a.Control.Usage(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, u)
}
func (a *API) power(signal pterodactyl.PowerSignal) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			a.fail(w, r, server.ErrNotFound)
			return
		}
		key, err := uuid.Parse(r.Header.Get("Idempotency-Key"))
		if err != nil {
			a.fail(w, r, server.ErrInvalid)
			return
		}
		var input struct {
			Confirmation string `json:"confirmation,omitempty"`
		}
		if err = decode(w, r, &input); err != nil {
			a.fail(w, r, err)
			return
		}
		if a.Control == nil {
			a.fail(w, r, control.ErrNotConfigured)
			return
		}
		if err = a.Control.Power(r.Context(), currentUser(r).ID, id, key, signal, input.Confirmation, requestID(r)); err != nil {
			a.fail(w, r, err)
			return
		}
		write(w, 202, map[string]string{"status": "accepted", "action": string(signal)})
	}
}
func (a *API) command(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		a.fail(w, r, server.ErrNotFound)
		return
	}
	key, err := uuid.Parse(r.Header.Get("Idempotency-Key"))
	if err != nil {
		a.fail(w, r, server.ErrInvalid)
		return
	}
	var input struct {
		Command string `json:"command"`
	}
	if err = decode(w, r, &input); err != nil {
		a.fail(w, r, err)
		return
	}
	if a.Control == nil {
		a.fail(w, r, control.ErrNotConfigured)
		return
	}
	if err = a.Control.Command(r.Context(), currentUser(r).ID, id, key, input.Command, requestID(r)); err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 202, map[string]string{"status": "accepted"})
}
func (a *API) ticket(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		a.fail(w, r, server.ErrNotFound)
		return
	}
	if a.Console == nil {
		a.fail(w, r, control.ErrNotConfigured)
		return
	}
	t, err := a.Console.Mint(r.Context(), currentUser(r).ID, id, bearer(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 201, t)
}
func (a *API) consoleStream(w http.ResponseWriter, r *http.Request) {
	// Browser authentication uses a one-use subprotocol ticket, never a session in a URL.
	if a.Console == nil {
		a.fail(w, r, control.ErrNotConfigured)
		return
	}
	if r.Header.Get("Origin") != a.Origin {
		write(w, 403, map[string]string{"error": "origin denied", "request_id": requestID(r)})
		return
	}
	protocols := strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",")
	token := ""
	hasProtocol := false
	for _, p := range protocols {
		p = strings.TrimSpace(p)
		if p == console.Protocol {
			hasProtocol = true
		}
		if strings.HasPrefix(p, "ticket.") {
			if token != "" {
				a.fail(w, r, console.ErrTicket)
				return
			}
			token = strings.TrimPrefix(p, "ticket.")
		}
	}
	if !hasProtocol || token == "" {
		a.fail(w, r, console.ErrTicket)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		a.fail(w, r, server.ErrNotFound)
		return
	}
	access, err := a.Console.Consume(r.Context(), id, token)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.Log.Info("console connecting", "request_id", requestID(r), "server_id", id, "user_id", access.UserID)
	if err = a.Console.Relay(w, r, access); err != nil {
		a.fail(w, r, err)
	}
}
