package httpapi

import (
	"net/http"

	"minecraft-hosting/internal/management"
)

func (a *API) whitelist(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	list, err := a.Management.Whitelist(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, list)
}

func (a *API) changeWhitelist(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	key, err := operationKey(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var input management.WhitelistChange
	if err = decode(w, r, &input); err != nil {
		a.fail(w, r, err)
		return
	}
	result, err := a.Management.ChangeWhitelist(r.Context(), currentUser(r).ID, id, key, input, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
