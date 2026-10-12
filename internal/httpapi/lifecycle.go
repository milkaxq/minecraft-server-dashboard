package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/server"
)

// The private beta has no checkout. Only a verified operator may allocate a
// server, and creation plus provisioning intent share one database transaction.
func (a *API) createBetaServer(w http.ResponseWriter, r *http.Request) {
	actor := currentUser(r).ID
	if err := a.Jobs.RequireAdmin(r.Context(), actor); err != nil {
		a.fail(w, r, err)
		return
	}
	key, err := uuid.Parse(r.Header.Get("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		a.fail(w, r, jobs.ErrInvalid)
		return
	}
	var input server.CreateInput
	if err = decode(w, r, &input); err != nil {
		a.fail(w, r, err)
		return
	}
	if err = input.Validate(); err != nil {
		a.fail(w, r, err)
		return
	}
	body, _ := json.Marshal(input)
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(body))
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "beta-create:"+actor.String()+":"+key.String()); err != nil {
		a.fail(w, r, err)
		return
	}
	var id uuid.UUID
	var prior string
	err = tx.QueryRow(r.Context(), "SELECT id,creation_fingerprint FROM servers WHERE owner_id=$1 AND creation_key=$2", actor, key).Scan(&id, &prior)
	if err == nil {
		if prior != fingerprint {
			a.fail(w, r, jobs.ErrConflict)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			a.fail(w, r, err)
			return
		}
		v, err := a.Servers.Get(r.Context(), actor, id)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		write(w, 200, v)
		return
	}
	if err != nil && err != pgx.ErrNoRows {
		a.fail(w, r, err)
		return
	}
	v, err := a.Servers.CreateTx(r.Context(), tx, actor, input, requestID(r))
	if err == nil {
		_, err = tx.Exec(r.Context(), "UPDATE servers SET creation_key=$2,creation_fingerprint=$3 WHERE id=$1", v.ID, key, fingerprint)
	}
	if err == nil {
		_, err = a.Jobs.EnqueueTx(r.Context(), tx, actor, v.ID, key, jobs.Provision, jobs.Input{Reason: "Private beta server creation"}, requestID(r))
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	queued, err := a.Servers.Get(r.Context(), actor, v.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 201, queued)
}

func (a *API) terminateOwnServer(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		a.fail(w, r, server.ErrNotFound)
		return
	}
	if err = a.Servers.Authorize(r.Context(), currentUser(r).ID, id, server.Delete); err != nil {
		a.fail(w, r, err)
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
	job, err := a.Jobs.Enqueue(r.Context(), currentUser(r).ID, id, key, jobs.Terminate, input, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 202, job)
}

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
