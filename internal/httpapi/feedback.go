package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"minecraft-hosting/internal/feedback"
	"minecraft-hosting/internal/server"
)

func (a *API) listFeedback(w http.ResponseWriter, r *http.Request)  { a.feedbackList(w, r, false) }
func (a *API) adminFeedback(w http.ResponseWriter, r *http.Request) { a.feedbackList(w, r, true) }
func (a *API) feedbackList(w http.ResponseWriter, r *http.Request, admin bool) {
	n, err := offset(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := (feedback.Service{DB: a.DB}).List(r.Context(), currentUser(r).ID, admin, n)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
func (a *API) createFeedback(w http.ResponseWriter, r *http.Request) {
	key, err := uuid.Parse(r.Header.Get("Idempotency-Key"))
	if err != nil {
		a.fail(w, r, server.ErrInvalid)
		return
	}
	var in feedback.Input
	if err = decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := (feedback.Service{DB: a.DB}).Create(r.Context(), currentUser(r).ID, key, in, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 201, v)
}
func (a *API) adminUpdateFeedback(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("report"))
	if err != nil {
		a.fail(w, r, server.ErrInvalid)
		return
	}
	var in feedback.Update
	if err = decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := (feedback.Service{DB: a.DB}).Update(r.Context(), currentUser(r).ID, id, in, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
