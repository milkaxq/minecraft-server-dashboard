package httpapi

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/management"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

func (a *API) managementRoutes(mux *http.ServeMux) {
	routes := map[string]http.HandlerFunc{
		"GET /servers/{id}/world":                     a.world,
		"GET /servers/{id}/world/download":            a.worldDownload,
		"POST /servers/{id}/world/backup":             a.worldBackup,
		"POST /servers/{id}/world/reset":              a.worldChange(false),
		"POST /servers/{id}/world/upload":             a.worldChange(true),
		"GET /servers/{id}/eula":                      a.eula,
		"POST /servers/{id}/eula":                     a.acceptEULA,
		"GET /servers/{id}/files":                     a.files,
		"GET /servers/{id}/files/text":                a.fileText,
		"POST /servers/{id}/files":                    a.changeFile,
		"POST /servers/{id}/files/upload":             a.uploadFile(false),
		"GET /servers/{id}/files/download":            a.download(false),
		"GET /servers/{id}/settings":                  a.settings,
		"PUT /servers/{id}/settings":                  a.saveSettings,
		"GET /servers/{id}/plugins":                   a.plugins,
		"POST /servers/{id}/plugins/upload":           a.uploadFile(true),
		"DELETE /servers/{id}/plugins":                a.deletePlugin,
		"GET /servers/{id}/backups":                   a.backups,
		"POST /servers/{id}/backups":                  a.newBackup,
		"POST /servers/{id}/backups/{backup}/restore": a.changeBackup("restore"),
		"DELETE /servers/{id}/backups/{backup}":       a.changeBackup("delete"),
		"GET /servers/{id}/backups/{backup}/download": a.download(true),
	}
	for route, handler := range routes {
		mux.HandleFunc(route, a.protected(a.liveLimit("management", 240, handler)))
	}
}
func (a *API) managementID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		a.fail(w, r, server.ErrNotFound)
		return id, false
	}
	if a.Management == nil {
		a.fail(w, r, control.ErrNotConfigured)
		return id, false
	}
	return id, true
}
func operationKey(r *http.Request) (uuid.UUID, error) {
	key, err := uuid.Parse(r.Header.Get("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		return key, server.ErrInvalid
	}
	return key, nil
}
func (a *API) files(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	dir := r.URL.Query().Get("path")
	if dir == "" {
		dir = "/"
	}
	list, err := a.Management.Files(r.Context(), currentUser(r).ID, id, dir)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, list)
}
func (a *API) fileText(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	file, err := a.Management.Text(r.Context(), currentUser(r).ID, id, r.URL.Query().Get("path"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, file)
}
func decodeManagement(w http.ResponseWriter, r *http.Request, input any) error {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		return auth.ErrInvalid
	}
	r.Body = http.MaxBytesReader(w, r.Body, (6<<20)+16384) // 1 MiB text may use JSON escapes.
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		return auth.ErrInvalid
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return auth.ErrInvalid
	}
	return nil
}
func (a *API) changeFile(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	key, err := operationKey(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in management.FileMutation
	if err = decodeManagement(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	result, err := a.Management.ChangeFile(r.Context(), currentUser(r).ID, id, key, in, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
func (a *API) uploadFile(plugin bool) http.HandlerFunc {
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
		// Authenticate/authorize before reading potentially large input.
		permission := server.Files
		if plugin {
			permission = server.Plugins
		}
		if err = a.Servers.Authorize(r.Context(), currentUser(r).ID, id, permission); err != nil {
			a.fail(w, r, err)
			return
		}
		if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/octet-stream" {
			a.fail(w, r, server.ErrInvalid)
			return
		}

		if r.ContentLength > pterodactyl.MaxUploadBytes {
			a.fail(w, r, server.ErrInvalid)
			return
		}
		// Bound aggregate buffering as well as the size of each upload.
		select {
		case a.uploadSlots <- struct{}{}:
			defer func() { <-a.uploadSlots }()
		default:
			a.fail(w, r, auth.ErrRateLimited)
			return
		}
		_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(2 * time.Minute))
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(3 * time.Minute))
		r.Body = http.MaxBytesReader(w, r.Body, pterodactyl.MaxUploadBytes)
		data, err := io.ReadAll(r.Body)
		if err != nil {
			a.fail(w, r, server.ErrInvalid)
			return
		}
		dir := r.URL.Query().Get("path")
		if plugin {
			dir = "/plugins"
		}
		result, err := a.Management.Upload(r.Context(), currentUser(r).ID, id, key, dir, r.URL.Query().Get("name"), data, plugin, requestID(r))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		write(w, 201, result)
	}
}
func (a *API) settings(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	in, err := a.Management.Settings(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, in)
}
func (a *API) saveSettings(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	key, err := operationKey(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in management.Settings
	if err = decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	result, err := a.Management.SaveSettings(r.Context(), currentUser(r).ID, id, key, in, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
func (a *API) plugins(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	list, err := a.Management.Plugins(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, list)
}
func (a *API) deletePlugin(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	key, err := operationKey(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in struct {
		Name         string `json:"name"`
		Confirmation string `json:"confirmation"`
	}
	if err = decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	result, err := a.Management.DeletePlugin(r.Context(), currentUser(r).ID, id, key, in.Name, in.Confirmation, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
func (a *API) backups(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	list, err := a.Management.Backups(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, list)
}
func (a *API) newBackup(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	key, err := operationKey(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err = decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	result, err := a.Management.NewBackup(r.Context(), currentUser(r).ID, id, key, in.Name, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 202, result)
}
func (a *API) changeBackup(action string) http.HandlerFunc {
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
		var in struct {
			Confirmation string `json:"confirmation"`
		}
		if err = decode(w, r, &in); err != nil {
			a.fail(w, r, err)
			return
		}
		result, err := a.Management.ChangeBackup(r.Context(), currentUser(r).ID, id, key, r.PathValue("backup"), action, in.Confirmation, requestID(r))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		write(w, 202, result)
	}
}
func (a *API) download(backup bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := a.managementID(w, r)
		if !ok {
			return
		}
		var stream io.ReadCloser
		var limit int64
		var err error
		file := r.URL.Query().Get("path")
		name := path.Base(file)
		if backup {
			stream, limit, err = a.Management.BackupDownload(r.Context(), currentUser(r).ID, id, r.PathValue("backup"))
			name = "backup-" + r.PathValue("backup") + ".tar.gz"
		} else {
			stream, limit, err = a.Management.Download(r.Context(), currentUser(r).ID, id, file)
		}
		if err != nil {
			a.fail(w, r, err)
			return
		}
		defer stream.Close()
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Minute))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if _, err = io.Copy(w, io.LimitReader(stream, limit)); err != nil {
			a.Log.Error("download interrupted", "request_id", requestID(r), "server_id", id)
		}
	}
}
