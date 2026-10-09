package httpapi

import (
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/management"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

func (a *API) world(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	result, err := a.Management.World(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
func (a *API) worldBackup(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
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
	result, err := a.Management.WorldBackup(r.Context(), currentUser(r).ID, id, key, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 202, result)
}
func (a *API) worldChange(upload bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := a.managementID(w, r)
		if !ok {
			return
		}
		key, err := operationKey(r)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		if err = a.Servers.Authorize(r.Context(), currentUser(r).ID, id, server.Worlds); err != nil {
			a.fail(w, r, err)
			return
		}
		var in management.WorldChange
		var data []byte
		if upload {
			in = management.WorldChange{Backup: r.URL.Query().Get("backup"), Confirmation: r.URL.Query().Get("confirmation"), Revision: r.URL.Query().Get("revision")}
			if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/octet-stream" || r.ContentLength > pterodactyl.MaxUploadBytes {
				a.fail(w, r, server.ErrInvalid)
				return
			}
			select {
			case a.uploadSlots <- struct{}{}:
				defer func() { <-a.uploadSlots }()
			default:
				a.fail(w, r, auth.ErrRateLimited)
				return
			}
			_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(2 * time.Minute))
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Minute))
			r.Body = http.MaxBytesReader(w, r.Body, pterodactyl.MaxUploadBytes)
			data, err = io.ReadAll(r.Body)
			if err != nil {
				a.fail(w, r, server.ErrInvalid)
				return
			}
		} else if err = decode(w, r, &in); err != nil {
			a.fail(w, r, err)
			return
		}
		result, err := a.Management.ChangeWorld(r.Context(), currentUser(r).ID, id, key, in, data, upload, requestID(r))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		write(w, 200, result)
	}
}
func (a *API) worldDownload(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	stream, name, err := a.Management.WorldDownload(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	defer stream.Close()
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Minute))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err = io.Copy(w, stream); err != nil {
		a.Log.Error("world download interrupted", "request_id", requestID(r), "server_id", id)
	}
}
