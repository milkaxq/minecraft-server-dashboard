package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"minecraft-hosting/internal/server"
)

func (a *API) memberRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /servers/{id}/members", a.protected(a.liveLimit("members.read", 180, a.listMembers)))
	for route, action := range map[string]string{"POST /servers/{id}/members": "add", "PUT /servers/{id}/members/{member}": "update", "DELETE /servers/{id}/members/{member}": "remove"} {
		mux.HandleFunc(route, a.protected(a.liveLimit("members.write", 60, a.changeMember(action))))
	}
}
func (a *API) listMembers(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		a.fail(w, r, server.ErrNotFound)
		return
	}
	result, err := a.Servers.ListMembers(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
func (a *API) changeMember(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			a.fail(w, r, server.ErrNotFound)
			return
		}
		if err = a.Servers.Authorize(r.Context(), currentUser(r).ID, id, server.Members); err != nil {
			a.fail(w, r, err)
			return
		}
		key, err := operationKey(r)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		target := uuid.Nil
		if action != "add" {
			target, err = uuid.Parse(r.PathValue("member"))
			if err != nil {
				a.fail(w, r, server.ErrNotFound)
				return
			}
		}
		var input server.MemberChange
		if err = decode(w, r, &input); err != nil {
			a.fail(w, r, err)
			return
		}
		result, err := a.Servers.ChangeMember(r.Context(), currentUser(r).ID, id, key, target, action, input, requestID(r))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		status := 200
		if action == "add" {
			status = 201
		}
		write(w, status, result)
	}
}
