package httpapi

import (
	"net/http"

	"minecraft-hosting/internal/management"
)

func (a *API) playerBans(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	result, err := a.Management.PlayerBans(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
func (a *API) changePlayer(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	key, err := operationKey(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in management.PlayerAction
	if err = decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	result, err := a.Management.ChangePlayer(r.Context(), currentUser(r).ID, id, key, in, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
