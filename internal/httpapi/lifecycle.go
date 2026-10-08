package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/server"
)

func (a *API) enqueue(action jobs.Action) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check administrator identity before interpreting target IDs or request bodies.
		if err := a.Jobs.RequireAdmin(r.Context(), currentUser(r).ID); err != nil {
			a.fail(w, r, err)
			return
		}
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			a.fail(w, r, server.ErrNotFound)
			return
		}
		key, err := uuid.Parse(r.Header.Get("Idempotency-Key"))
		if err != nil {
			a.fail(w, r, jobs.ErrInvalid)
			return
		}
		var input jobs.Input
		if err = decode(w, r, &input); err != nil {
			a.fail(w, r, err)
			return
		}
		job, err := a.Jobs.Enqueue(r.Context(), currentUser(r).ID, id, key, action, input, requestID(r))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		write(w, 202, job)
	}
}
func (a *API) getJob(w http.ResponseWriter, r *http.Request) {
	if err := a.Jobs.RequireAdmin(r.Context(), currentUser(r).ID); err != nil {
		a.fail(w, r, err)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		a.fail(w, r, server.ErrNotFound)
		return
	}
	j, err := a.Jobs.Get(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, j)
}
func (a *API) softwareOptions(w http.ResponseWriter, r *http.Request) {
	v, err := a.Servers.SoftwareOptions(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
