package httpapi

import (
	"errors"
	"net/http"
	"time"

	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/playerjoin"
)

func (a *API) playerJoin() playerjoin.Service {
	s := playerjoin.Service{DB: a.DB, Management: a.Management}
	if a.Management != nil {
		s.Catalog = a.Management.Catalog
	}
	return s
}
func (a *API) playerJoinRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /servers/{id}/player-invite", a.protected(a.liveLimit("player-invite", 30, a.getPlayerInvite)))
	mux.HandleFunc("POST /servers/{id}/player-invite", a.protected(a.liveLimit("player-invite", 10, a.publishPlayerInvite)))
	mux.HandleFunc("DELETE /servers/{id}/player-invite", a.protected(a.liveLimit("player-invite", 10, a.revokePlayerInvite)))
	mux.HandleFunc("GET /join/{slug}", a.publicPlayerJoin(false))
	mux.HandleFunc("GET /join/{slug}/pack", a.publicPlayerJoin(true))
}
func (a *API) getPlayerInvite(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	result, err := a.playerJoin().Get(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
func (a *API) publishPlayerInvite(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	select {
	case a.uploadSlots <- struct{}{}:
		defer func() { <-a.uploadSlots }()
	default:
		a.fail(w, r, auth.ErrRateLimited)
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(time.Minute))
	result, err := a.playerJoin().Publish(r.Context(), currentUser(r).ID, id, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
func (a *API) revokePlayerInvite(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	if err := a.playerJoin().Revoke(r.Context(), currentUser(r).ID, id, requestID(r)); err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]bool{"revoked": true})
}
func (a *API) publicPlayerJoin(pack bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		// Proxy headers are never trusted as identity. A bounded shared public budget
		// is intentional for beta; these routes query snapshots, never Wings or files.
		if err := a.Auth.RateLimitWindow(r.Context(), "public-player-join", 120, time.Minute); err != nil {
			w.Header().Set("Retry-After", "60")
			a.fail(w, r, err)
			return
		}
		if pack {
			data, err := a.playerJoin().Pack(r.Context(), r.PathValue("slug"))
			if err != nil {
				if errors.Is(err, playerjoin.ErrUnavailable) {
					a.fail(w, r, control.ErrUnavailable)
				} else {
					a.fail(w, r, err)
				}
				return
			}
			w.Header().Set("Content-Type", "application/x-modrinth-modpack+zip")
			w.Header().Set("Content-Disposition", `attachment; filename="blockyard-player-setup.mrpack"`)
			w.WriteHeader(200)
			_, _ = w.Write(data)
			return
		}
		result, err := a.playerJoin().Public(r.Context(), r.PathValue("slug"))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		write(w, 200, result)
	}
}
