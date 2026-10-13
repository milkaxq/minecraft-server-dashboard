package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/server"
)

func (a *API) notifications(w http.ResponseWriter, r *http.Request) {
	items, err := a.Servers.Notifications(r.Context(), currentUser(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, items)
}
func (a *API) readNotification(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("notification"))
	if err != nil {
		a.fail(w, r, server.ErrNotFound)
		return
	}
	if err = a.Servers.ReadNotification(r.Context(), currentUser(r).ID, id); err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]bool{"read": true})
}

func (a *API) activity(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	var before *time.Time
	if raw := r.URL.Query().Get("before"); raw != "" {
		v, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			a.fail(w, r, server.ErrInvalid)
			return
		}
		before = &v
	}
	result, err := a.Servers.Activity(r.Context(), currentUser(r).ID, id, before)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
func (a *API) storage(w http.ResponseWriter, r *http.Request) {
	select {
	case a.uploadSlots <- struct{}{}:
		defer func() { <-a.uploadSlots }()
	default:
		a.fail(w, r, auth.ErrRateLimited)
		return
	}
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	result, err := a.Management.Storage(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}

func (a *API) catalogUpdates(w http.ResponseWriter, r *http.Request) {
	select {
	case a.uploadSlots <- struct{}{}:
		defer func() { <-a.uploadSlots }()
	default:
		a.fail(w, r, auth.ErrRateLimited)
		return
	}
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	result, err := a.Management.CatalogUpdates(r.Context(), currentUser(r).ID, id, r.PathValue("collection"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
