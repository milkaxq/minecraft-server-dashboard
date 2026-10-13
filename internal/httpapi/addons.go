package httpapi

import (
	"net/http"

	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/management"
	"minecraft-hosting/internal/server"
)

func (a *API) voiceChat(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	v, err := a.Management.VoiceChat(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
func (a *API) saveVoiceChat(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	if err := (server.Service{DB: a.DB}).Authorize(r.Context(), currentUser(r).ID, id, server.Settings); err != nil {
		a.fail(w, r, err)
		return
	}
	key, err := operationKey(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var input management.VoiceInput
	if err = decode(w, r, &input); err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.Management.SaveVoiceChat(r.Context(), currentUser(r).ID, id, key, input, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
func (a *API) prepareVoicePort(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	if err := (server.Service{DB: a.DB}).Authorize(r.Context(), currentUser(r).ID, id, server.Settings); err != nil {
		a.fail(w, r, err)
		return
	}
	key, err := operationKey(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var input struct{}
	if err = decode(w, r, &input); err != nil {
		a.fail(w, r, err)
		return
	}
	// Check addon access before contacting Panel or enqueuing privileged work.
	if err = (server.Service{DB: a.DB}).Authorize(r.Context(), currentUser(r).ID, id, server.Settings); err != nil {
		a.fail(w, r, err)
		return
	}
	config, err := a.Management.VoiceChat(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if !config.Installed {
		a.fail(w, r, server.ErrInvalid)
		return
	}
	v, err := a.Jobs.Enqueue(r.Context(), currentUser(r).ID, id, key, jobs.VoicePort, jobs.Input{Reason: "Prepare Simple Voice Chat networking"}, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 202, v)
}

func (a *API) voiceNetwork(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	v, err := a.Management.VoiceNetworkStatus(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}

func (a *API) clientRequirements(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	v, err := a.Management.ClientRequirements(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, http.StatusOK, v)
}
