package httpapi

import (
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"minecraft-hosting/internal/auth"
)

func (a *API) adminInvitations(w http.ResponseWriter, r *http.Request) {
	list, err := a.Auth.Invitations(r.Context(), currentUser(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, list)
}
func (a *API) adminCreateInvitation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string `json:"email"`
	}
	if err := decode(w, r, &input); err != nil {
		a.fail(w, r, err)
		return
	}
	invite, err := a.Auth.CreateInvitation(r.Context(), currentUser(r).ID, input.Email, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	fragment := url.Values{"invite": {invite.Token}, "email": {invite.Email}}.Encode()
	// Return the secret link only once. Lists and audit entries never contain it.
	write(w, 201, struct {
		ID        uuid.UUID `json:"id"`
		Email     string    `json:"email"`
		Link      string    `json:"link"`
		ExpiresAt time.Time `json:"expires_at"`
	}{invite.ID, invite.Email, a.Origin + "/register#" + fragment, invite.ExpiresAt})
}
func (a *API) adminRevokeInvitation(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("invite"))
	if err != nil {
		a.fail(w, r, auth.ErrInvalid)
		return
	}
	if err = a.Auth.RevokeInvitation(r.Context(), currentUser(r).ID, id, requestID(r)); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
