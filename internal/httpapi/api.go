package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/admin"
	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/billing"
	"minecraft-hosting/internal/console"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/management"
	"minecraft-hosting/internal/plan"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
	"minecraft-hosting/internal/user"
)

type contextKey int

const (
	userKey contextKey = iota
	requestKey
)

type API struct {
	DB                 *pgxpool.Pool
	Auth               *auth.Service
	Billing            *billing.Service
	Admin              *admin.Service
	Plans              plan.Service
	Servers            server.Service
	Jobs               jobs.Service
	Control            *control.Service
	Management         *management.Service
	Console            *console.Service
	Log                *slog.Logger
	Origin             string
	uploadSlots        chan struct{}
	requests, failures atomic.Uint64
}

func (a *API) Handler() http.Handler {
	if a.uploadSlots == nil {
		a.uploadSlots = make(chan struct{}, 2)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := a.DB.Ping(ctx); err != nil {
			write(w, 503, map[string]string{"status": "unavailable"})
			return
		}
		write(w, 200, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("POST /auth/register", a.limited(a.register))
	mux.HandleFunc("POST /auth/login", a.limited(a.login))
	mux.HandleFunc("POST /auth/forgot-password", a.limited(a.forgotPassword))
	mux.HandleFunc("POST /auth/reset-password", a.limited(a.resetPassword))
	mux.HandleFunc("PUT /account/password", a.protected(a.changePassword))
	mux.HandleFunc("POST /auth/verify-email", a.limited(a.verify))
	mux.HandleFunc("POST /auth/resend-verification", a.limited(a.resend))
	mux.HandleFunc("POST /auth/logout", a.protected(a.logout))
	mux.HandleFunc("GET /account", a.protected(func(w http.ResponseWriter, r *http.Request) { write(w, 200, currentUser(r)) }))
	mux.HandleFunc("GET /plans", a.plans)
	mux.HandleFunc("GET /servers", a.protected(a.listServers))
	mux.HandleFunc("POST /servers", a.protected(a.createServer))
	mux.HandleFunc("GET /servers/{id}", a.protected(a.getServer))
	mux.HandleFunc("GET /minecraft/options", a.softwareOptions)
	mux.HandleFunc("GET /servers/{id}/usage", a.protected(a.usage))
	for _, signal := range []pterodactyl.PowerSignal{pterodactyl.PowerStart, pterodactyl.PowerStop, pterodactyl.PowerRestart, pterodactyl.PowerKill} {
		mux.HandleFunc("POST /servers/{id}/"+string(signal), a.protected(a.liveLimit("power", 60, a.power(signal))))
	}
	mux.HandleFunc("POST /servers/{id}/commands", a.protected(a.liveLimit("commands", 300, a.command)))
	mux.HandleFunc("POST /servers/{id}/console-ticket", a.protected(a.liveLimit("tickets", 60, a.ticket)))
	mux.HandleFunc("GET /servers/{id}/console", a.consoleStream)
	a.memberRoutes(mux)
	a.managementRoutes(mux)
	a.billingRoutes(mux)
	a.adminRoutes(mux)
	if a.Jobs.DB != nil {
		mux.HandleFunc("POST /admin/servers/{id}/provision", a.protected(a.enqueue(jobs.Provision)))
		mux.HandleFunc("POST /admin/servers/{id}/retry", a.protected(a.enqueue(jobs.Provision)))
		mux.HandleFunc("POST /admin/servers/{id}/suspend", a.protected(a.enqueue(jobs.Suspend)))
		mux.HandleFunc("POST /admin/servers/{id}/unsuspend", a.protected(a.enqueue(jobs.Unsuspend)))
		mux.HandleFunc("POST /admin/servers/{id}/terminate", a.protected(a.enqueue(jobs.Terminate)))
		mux.HandleFunc("GET /admin/jobs/{id}", a.protected(a.getJob))
	}
	return a.middleware(mux)
}
func currentUser(r *http.Request) user.User { return r.Context().Value(userKey).(user.User) }
func requestID(r *http.Request) string      { return r.Context().Value(requestKey).(string) }
func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}
func (a *API) protected(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := a.Auth.Authenticate(r.Context(), bearer(r))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	}
}
func (a *API) limited(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if err = a.Auth.RateLimit(r.Context(), "auth:"+ip, 30); err != nil {
			a.fail(w, r, err)
			return
		}
		next(w, r)
	}
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		return auth.ErrInvalid
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return auth.ErrInvalid
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		return auth.ErrInvalid
	}
	return nil
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (a *API) fail(w http.ResponseWriter, r *http.Request, err error) {
	status, message := 500, "internal server error"
	switch {
	case errors.Is(err, auth.ErrResetToken):
		status, message = 400, "reset link is invalid or expired; request a new one"
	case errors.Is(err, auth.ErrCurrentPassword):
		status, message = 400, "current password is incorrect"
	case errors.Is(err, auth.ErrRecoveryUnavailable):
		status, message = 503, "account recovery is temporarily unavailable"
	case errors.Is(err, server.ErrMemberConflict):
		status, message = 409, "member already exists, the 20-member limit is reached, or another operation is in progress"
	case errors.Is(err, server.ErrMemberUnavailable):
		status, message = 400, "ask this person to register and verify their email first, then check the email address"
	case errors.Is(err, admin.ErrConflict):
		status, message = 409, "another plan already uses this name"
	case errors.Is(err, billing.ErrSignature):
		status, message = 401, "invalid payment signature"
	case errors.Is(err, billing.ErrUnavailable):
		status, message = 503, "checkout is not configured"
	case errors.Is(err, billing.ErrConflict):
		status, message = 409, "billing operation conflicts with the current order or subscription"
	case errors.Is(err, auth.ErrInvalid), errors.Is(err, server.ErrInvalid), errors.Is(err, jobs.ErrInvalid):
		status, message = 400, "invalid request"
	case errors.Is(err, console.ErrTicket):
		status, message = 401, "console session expired"
	case errors.Is(err, control.ErrNotConfigured):
		status, message = 503, "live management is not configured"
	case errors.Is(err, management.ErrOffline):
		status, message = 409, "stop the server before changing files, settings, plugins or restoring a backup"
	case errors.Is(err, management.ErrLimit):
		status, message = 409, "your backup allowance is full; delete a backup before creating another"
	case errors.Is(err, management.ErrChanged):
		status, message = 409, "file changed or already exists; reload before saving"
	case errors.Is(err, management.ErrPlugin):
		status, message = 400, "upload a compatible plugin .jar; Paper-only plugins require Paper or Purpur"
	case errors.Is(err, management.ErrMod):
		status, message = 400, "upload a server mod .jar for your chosen loader; other loaders and client-only Fabric mods are incompatible"
	case errors.Is(err, control.ErrState):
		status, message = 409, "server is not available for live management"
	case errors.Is(err, control.ErrEULA):
		status, message = 409, "the server owner must accept the Minecraft EULA before starting"
	case errors.Is(err, control.ErrBusy):
		status, message = 409, "another operation is in progress"
	case errors.Is(err, control.ErrCommand):
		status, message = 403, "command is not allowed for your server role"
	case errors.Is(err, control.ErrUnknown):
		status, message = 502, "request outcome could not be confirmed; check server status before retrying"
	case errors.Is(err, control.ErrUnavailable):
		status, message = 502, "game server connection is unavailable"
	case errors.Is(err, auth.ErrCredentials):
		status, message = 401, "invalid credentials"
	case errors.Is(err, jobs.ErrForbidden):
		status, message = 403, "administrator access required"
	case errors.Is(err, jobs.ErrConflict):
		status, message = 409, "operation conflicts with current state"
	case errors.Is(err, auth.ErrUnverified):
		status, message = 403, "verify your email before login"
	case errors.Is(err, auth.ErrDuplicate):
		status, message = 409, "email already registered"
	case errors.Is(err, server.ErrNotFound), errors.Is(err, plan.ErrUnavailable):
		status, message = 404, "resource not found"
	case errors.Is(err, auth.ErrRateLimited):
		status, message = 429, "too many requests"
		w.Header().Set("Retry-After", "900")
	}
	// Never serialize database errors, credentials, request bodies or SMTP responses.
	if status == 500 {
		a.Log.Error("request failed", "request_id", requestID(r), "category", "internal")
	}
	write(w, status, map[string]string{"error": message, "request_id": requestID(r)})
}
func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var input auth.RegisterInput
	if err := decode(w, r, &input); err != nil {
		a.fail(w, r, err)
		return
	}
	u, err := a.Auth.Register(r.Context(), input, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 201, u)
}
func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(w, r, &input); err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.Auth.Login(r.Context(), input.Email, input.Password, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
func (a *API) verify(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token string `json:"token"`
	}
	if err := decode(w, r, &input); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.Auth.Verify(r.Context(), input.Token, requestID(r)); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(204)
}
func (a *API) resend(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string `json:"email"`
	}
	if err := decode(w, r, &input); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.Auth.Resend(r.Context(), input.Email); err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 202, map[string]string{"message": "If an unverified account exists, a verification email has been sent."})
}
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if err := a.Auth.Logout(r.Context(), bearer(r)); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(204)
}
func (a *API) plans(w http.ResponseWriter, r *http.Request) {
	v, err := a.Plans.List(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
func (a *API) listServers(w http.ResponseWriter, r *http.Request) {
	v, err := a.Servers.List(r.Context(), currentUser(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
func (a *API) createServer(w http.ResponseWriter, r *http.Request) {
	var input server.CreateInput
	if err := decode(w, r, &input); err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.Servers.Create(r.Context(), currentUser(r).ID, input, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 201, v)
}
func (a *API) getServer(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		a.fail(w, r, server.ErrNotFound)
		return
	}
	v, err := a.Servers.Get(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	role, err := a.Servers.Role(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, struct {
		server.Server
		Role                server.Role         `json:"role"`
		Permissions         []server.Permission `json:"permissions"`
		ManagementAvailable bool                `json:"management_available"`
		ConsoleAvailable    bool                `json:"console_available"`
	}{v, role, server.Permissions(role), a.Control != nil && a.Control.Panel != nil, a.Console.Configured()})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(s int) {
	if w.status != 0 {
		return
	}
	w.status = s
	w.ResponseWriter.WriteHeader(s)
}
func (w *responseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(p)
}
func (a *API) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := uuid.NewString()
		r = r.WithContext(context.WithValue(r.Context(), requestKey, id))
		rw := &responseWriter{ResponseWriter: w}
		rw.Header().Set("X-Request-ID", id)
		rw.Header().Set("X-Content-Type-Options", "nosniff")
		rw.Header().Set("Cache-Control", "no-store")
		rw.Header().Set("Referrer-Policy", "no-referrer")
		defer func() {
			if recover() != nil {
				a.Log.Error("handler panic", "request_id", id)
				if rw.status == 0 {
					write(rw, 500, map[string]string{"error": "internal server error", "request_id": id})
				}
			}
			a.requests.Add(1)
			if rw.status >= 500 {
				a.failures.Add(1)
			}
			a.Log.Info("http request", "request_id", id, "method", r.Method, "route", r.Pattern, "status", rw.status, "duration_ms", time.Since(start).Milliseconds())
		}()
		origin := r.Header.Get("Origin")
		if origin != "" {
			if origin != a.Origin {
				write(rw, 403, map[string]string{"error": "origin denied", "request_id": id})
				return
			}
			rw.Header().Set("Access-Control-Allow-Origin", origin)
			rw.Header().Set("Vary", "Origin")
			rw.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
		}
		if r.Method == http.MethodOptions {
			rw.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
			rw.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,Idempotency-Key")
			rw.WriteHeader(204)
			return
		}
		next.ServeHTTP(rw, r)
	})
}

// Metrics is intentionally on a separate private listener, not the public API.
func (a *API) Metrics() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = io.WriteString(w, "# TYPE hosting_http_requests_total counter\nhosting_http_requests_total "+strconv.FormatUint(a.requests.Load(), 10)+"\n# TYPE hosting_http_failures_total counter\nhosting_http_failures_total "+strconv.FormatUint(a.failures.Load(), 10)+"\n")
	})
}

func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil {
		w.status = http.StatusSwitchingProtocols
	}
	return conn, rw, err
}
func (w *responseWriter) Flush() { _ = http.NewResponseController(w.ResponseWriter).Flush() }
