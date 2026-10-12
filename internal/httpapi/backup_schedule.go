package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"minecraft-hosting/internal/backups"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/server"
)

func (a *API) backupScheduleID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		a.fail(w, r, server.ErrInvalid)
		return uuid.Nil, false
	}
	if a.BackupSchedules == nil {
		a.fail(w, r, control.ErrNotConfigured)
		return uuid.Nil, false
	}
	return id, true
}
func (a *API) backupSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := a.backupScheduleID(w, r)
	if !ok {
		return
	}
	v, err := a.BackupSchedules.Get(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
func (a *API) saveBackupSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := a.backupScheduleID(w, r)
	if !ok {
		return
	}
	var in backups.Update
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.BackupSchedules.Put(r.Context(), currentUser(r).ID, id, in, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
