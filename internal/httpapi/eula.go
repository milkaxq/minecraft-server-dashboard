package httpapi

import "net/http"

func (a *API) eula(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	status, err := a.Management.EULA(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, status)
}

func (a *API) acceptEULA(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	key, err := operationKey(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var input struct {
		Accepted bool `json:"accepted"`
	}
	if err = decode(w, r, &input); err != nil {
		a.fail(w, r, err)
		return
	}
	status, err := a.Management.AcceptEULA(r.Context(), currentUser(r).ID, id, key, input.Accepted, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, status)
}
