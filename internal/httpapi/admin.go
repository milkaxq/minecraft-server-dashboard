package httpapi

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/plan"
	"minecraft-hosting/internal/server"
)

func (a *API) adminRoutes(mux *http.ServeMux) {
	routes := map[string]http.HandlerFunc{"GET /admin/invitations": a.adminInvitations, "POST /admin/invitations": a.adminCreateInvitation, "POST /admin/invitations/{invite}/revoke": a.adminRevokeInvitation, "GET /admin/plans": a.adminPlans, "POST /admin/plans": a.adminSavePlan, "PUT /admin/plans/{plan}": a.adminSavePlan, "GET /admin/users": a.adminUsers, "GET /admin/servers": a.adminServers, "GET /admin/orders": a.adminOrders, "GET /admin/nodes": a.adminNodes, "GET /admin/audit": a.adminAudit}
	for route, handler := range routes {
		mux.HandleFunc(route, a.protected(a.liveLimit("admin", 240, func(w http.ResponseWriter, r *http.Request) {
			if a.Admin == nil {
				a.fail(w, r, control.ErrNotConfigured)
				return
			}
			if err := a.Admin.Require(r.Context(), currentUser(r).ID); err != nil {
				a.fail(w, r, err)
				return
			}
			handler(w, r)
		})))
	}
}
func (a *API) adminPlans(w http.ResponseWriter, r *http.Request) {
	v, err := a.Admin.Plans(r.Context(), currentUser(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
func (a *API) adminSavePlan(w http.ResponseWriter, r *http.Request) {
	id := uuid.Nil
	var err error
	if r.Method == "PUT" {
		id, err = uuid.Parse(r.PathValue("plan"))
		if err != nil {
			a.fail(w, r, server.ErrNotFound)
			return
		}
	}
	var input plan.Plan
	if err = decode(w, r, &input); err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.Admin.SavePlan(r.Context(), currentUser(r).ID, id, input, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	status := 200
	if r.Method == "POST" {
		status = 201
	}
	write(w, status, v)
}
func offset(r *http.Request) (int, error) {
	value := r.URL.Query().Get("offset")
	if value == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 || n > 100000 {
		return 0, server.ErrInvalid
	}
	return n, nil
}
func (a *API) adminUsers(w http.ResponseWriter, r *http.Request) {
	n, err := offset(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.Admin.Users(r.Context(), currentUser(r).ID, n)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
func (a *API) adminServers(w http.ResponseWriter, r *http.Request) {
	n, err := offset(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.Admin.Servers(r.Context(), currentUser(r).ID, n)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
func (a *API) adminOrders(w http.ResponseWriter, r *http.Request) {
	n, err := offset(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.Admin.Orders(r.Context(), currentUser(r).ID, n)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
func (a *API) adminNodes(w http.ResponseWriter, r *http.Request) {
	v, err := a.Admin.Nodes(r.Context(), currentUser(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
func (a *API) adminAudit(w http.ResponseWriter, r *http.Request) {
	n, err := offset(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.Admin.Audit(r.Context(), currentUser(r).ID, n)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
