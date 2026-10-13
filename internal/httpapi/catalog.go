package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"minecraft-hosting/internal/catalog"
	"minecraft-hosting/internal/server"

	"minecraft-hosting/internal/auth"
)

func (a *API) catalogSearch(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	offset := 0
	if raw := q.Get("offset"); raw != "" {
		var err error
		offset, err = strconv.Atoi(raw)
		if err != nil {
			a.fail(w, r, server.ErrInvalid)
			return
		}
	}
	v, err := a.Management.CatalogSearch(r.Context(), currentUser(r).ID, id, r.PathValue("collection"), catalog.SearchOptions{Query: q.Get("q"), Sort: q.Get("sort"), Category: q.Get("category"), Offset: offset})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
func (a *API) catalogPreview(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	v, err := a.Management.CatalogPreview(r.Context(), currentUser(r).ID, id, r.PathValue("collection"), r.PathValue("project"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
func (a *API) catalogInstall(w http.ResponseWriter, r *http.Request) {
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
		Revision string `json:"revision"`
	}
	if err = decode(w, r, &input); err != nil {
		a.fail(w, r, err)
		return
	}
	select {
	case a.uploadSlots <- struct{}{}:
		defer func() { <-a.uploadSlots }()
	default:
		a.fail(w, r, auth.ErrRateLimited)
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(3 * time.Minute))
	v, err := a.Management.CatalogInstall(r.Context(), currentUser(r).ID, id, key, r.PathValue("collection"), r.PathValue("project"), input.Revision, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}

func (a *API) catalogInstalled(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	v, err := a.Management.CatalogInstalled(r.Context(), currentUser(r).ID, id, r.PathValue("collection"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
func (a *API) catalogProgress(w http.ResponseWriter, r *http.Request) {
	id, ok := a.managementID(w, r)
	if !ok {
		return
	}
	key, err := uuid.Parse(r.PathValue("operation"))
	if err != nil {
		a.fail(w, r, server.ErrInvalid)
		return
	}
	v, err := a.Management.CatalogProgress(r.Context(), currentUser(r).ID, id, key, r.PathValue("collection"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
