package pterodactyl

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// APIError deliberately omits remote response bodies and URLs, which may contain secrets.
type APIError struct{ Status int }

func (e *APIError) Error() string { return fmt.Sprintf("panel HTTP status %d", e.Status) }
func (e *APIError) Is(target error) bool {
	return (e.Status == 404 && target == ErrNotFound) || ((e.Status == 409 || e.Status == 422) && target == ErrConflict)
}

// Rejected reports a definitive non-creation response. Timeouts, redirects and 5xx
// are ambiguous; callers must reconcile rather than repeat a create request.
func Rejected(err error) bool {
	var e *APIError
	return errors.As(err, &e) && (e.Status == 400 || e.Status == 401 || e.Status == 403 || e.Status == 404 || e.Status == 409 || e.Status == 422 || e.Status == 429)
}

type transport struct {
	base, key string
	http      *http.Client
}

func newTransport(base, key string, allowHTTP bool) (transport, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(allowHTTP && u.Scheme == "http")) || key == "" || strings.ContainsAny(key, "\r\n") {
		return transport{}, ErrInvalid
	}
	return transport{base: strings.TrimRight(base, "/"), key: key, http: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (t transport) request(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return ErrInvalid
		}
		body = bytes.NewReader(data)
	}
	r, err := http.NewRequestWithContext(ctx, method, t.base+path, body)
	if err != nil {
		return ErrInvalid
	}
	r.Header.Set("Authorization", "Bearer "+t.key)
	r.Header.Set("Accept", "application/vnd.pterodactyl.v1+json")
	r.Header.Set("Content-Type", "application/json")
	response, err := t.http.Do(r)
	if err != nil {
		return fmt.Errorf("panel transport failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &APIError{Status: response.StatusCode}
	}
	if output == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return err
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return fmt.Errorf("panel response unreadable")
	}
	if err = json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("panel response invalid")
	}
	return nil
}

type ApplicationHTTP struct{ transport }

func NewApplication(base, key string, allowHTTP bool) (*ApplicationHTTP, error) {
	t, err := newTransport(base, key, allowHTTP)
	if err != nil {
		return nil, err
	}
	return &ApplicationHTTP{t}, nil
}

var _ ApplicationClient = (*ApplicationHTTP)(nil)

type userAttributes struct {
	ID         int       `json:"id"`
	ExternalID uuid.UUID `json:"external_id"`
}

func (a *ApplicationHTTP) FindUserByExternalID(ctx context.Context, id uuid.UUID) (User, error) {
	var r struct {
		Attributes userAttributes `json:"attributes"`
	}
	err := a.request(ctx, "GET", "/api/application/users/external/"+id.String(), nil, &r)
	if err != nil {
		return User{}, err
	}
	if r.Attributes.ID <= 0 || r.Attributes.ExternalID != id {
		return User{}, ErrInvalid
	}
	return User{ID: r.Attributes.ID, ExternalID: r.Attributes.ExternalID}, nil
}
func (a *ApplicationHTTP) CreateUser(ctx context.Context, r UserRequest) (User, error) {
	if r.ExternalID == uuid.Nil || r.Email == "" {
		return User{}, ErrInvalid
	}
	// The customer never receives Panel login credentials. All game-server access is mediated by this API.
	password := make([]byte, 48)
	if _, err := rand.Read(password); err != nil {
		return User{}, err
	}
	payload := map[string]any{"external_id": r.ExternalID.String(), "username": "mc_" + strings.ReplaceAll(r.ExternalID.String(), "-", ""), "email": r.Email, "first_name": r.Name, "last_name": "Customer", "password": base64.RawURLEncoding.EncodeToString(password), "language": "en", "root_admin": false}
	var response struct {
		Attributes userAttributes `json:"attributes"`
	}
	if err := a.request(ctx, "POST", "/api/application/users", payload, &response); err != nil {
		return User{}, err
	}
	if response.Attributes.ID <= 0 || response.Attributes.ExternalID != r.ExternalID {
		return User{}, ErrInvalid
	}
	return User{ID: response.Attributes.ID, ExternalID: r.ExternalID}, nil
}

type serverAttributes struct {
	ID         int       `json:"id"`
	Identifier string    `json:"identifier"`
	ExternalID uuid.UUID `json:"external_id"`
	Suspended  bool      `json:"suspended"`
	Status     string    `json:"status"`
	Node       int       `json:"node"`
	User       int       `json:"user"`
	Allocation int       `json:"allocation"`
	Name       string    `json:"name"`
	Egg        int       `json:"egg"`
	Limits     struct {
		Memory int `json:"memory"`
		Disk   int `json:"disk"`
		CPU    int `json:"cpu"`
	} `json:"limits"`
	Features struct {
		Backups   int `json:"backups"`
		Databases int `json:"databases"`
	} `json:"feature_limits"`
	Container struct {
		Installed int `json:"installed"`
	} `json:"container"`
}

func (r serverAttributes) model() Server {
	// Panel v2 prefers status over its deprecated suspended/installed fields.
	failed := r.Status == "install_failed" || r.Status == "reinstall_failed"
	installed := r.Container.Installed == 1 && r.Status != "installing" && !failed
	return Server{ID: r.ID, Identifier: r.Identifier, ExternalID: r.ExternalID, Suspended: r.Suspended || r.Status == "suspended", Installed: installed, InstallFailed: failed, Request: ServerRequest{ExternalID: r.ExternalID, UserID: r.User, NodeID: r.Node, AllocationID: r.Allocation, EggID: r.Egg, Name: r.Name, Limits: Limits{RAMMB: r.Limits.Memory, DiskMB: r.Limits.Disk, CPU: r.Limits.CPU, Backups: r.Features.Backups, Databases: r.Features.Databases}}}
}
func (a *ApplicationHTTP) server(ctx context.Context, path string) (Server, error) {
	var r struct {
		Attributes serverAttributes `json:"attributes"`
	}
	err := a.request(ctx, "GET", path, nil, &r)
	if err != nil {
		return Server{}, err
	}
	if r.Attributes.ID <= 0 || r.Attributes.Identifier == "" || r.Attributes.ExternalID == uuid.Nil {
		return Server{}, ErrInvalid
	}
	return r.Attributes.model(), nil
}
func (a *ApplicationHTTP) GetServer(ctx context.Context, id int) (Server, error) {
	return a.server(ctx, "/api/application/servers/"+strconv.Itoa(id))
}
func (a *ApplicationHTTP) FindServerByExternalID(ctx context.Context, id uuid.UUID) (Server, error) {
	s, err := a.server(ctx, "/api/application/servers/external/"+id.String())
	if err == nil && s.ExternalID != id {
		return Server{}, ErrInvalid
	}
	return s, err
}
func (a *ApplicationHTTP) CreateServer(ctx context.Context, r ServerRequest) (Server, error) {
	if err := ValidateRequest(r); err != nil {
		return Server{}, err
	}
	payload := map[string]any{"external_id": r.ExternalID.String(), "name": r.Name, "user": r.UserID, "egg": r.EggID, "docker_image": r.Image, "startup": r.Startup, "environment": r.Environment, "limits": map[string]int{"memory": r.Limits.RAMMB, "swap": 0, "disk": r.Limits.DiskMB, "io": 500, "cpu": r.Limits.CPU}, "feature_limits": map[string]int{"databases": r.Limits.Databases, "allocations": 0, "backups": r.Limits.Backups}, "allocation": map[string]int{"default": r.AllocationID}, "start_on_completion": false, "skip_scripts": false, "oom_disabled": false}
	var response struct {
		Attributes serverAttributes `json:"attributes"`
	}
	if err := a.request(ctx, "POST", "/api/application/servers", payload, &response); err != nil {
		return Server{}, err
	}
	s := response.Attributes.model()
	if s.ID <= 0 || s.Identifier == "" || s.ExternalID != r.ExternalID {
		return Server{}, ErrInvalid
	}
	return s, nil
}
func (a *ApplicationHTTP) DeleteServer(ctx context.Context, id int) error {
	err := a.request(ctx, "DELETE", "/api/application/servers/"+strconv.Itoa(id), nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
func (a *ApplicationHTTP) SuspendServer(ctx context.Context, id int) error {
	return a.request(ctx, "POST", "/api/application/servers/"+strconv.Itoa(id)+"/suspend", nil, nil)
}
func (a *ApplicationHTTP) UnsuspendServer(ctx context.Context, id int) error {
	return a.request(ctx, "POST", "/api/application/servers/"+strconv.Itoa(id)+"/unsuspend", nil, nil)
}

type Allocation struct {
	ID       int
	Hostname string
	Port     int
	Assigned bool
}

// ListAllocations is operator-only discovery, not customer infrastructure access.
func (a *ApplicationHTTP) ListAllocations(ctx context.Context, node int) ([]Allocation, error) {
	result := []Allocation{}
	for page := 1; page <= 1000; page++ {
		var r struct {
			Data []struct {
				Attributes struct {
					ID       int     `json:"id"`
					IP       string  `json:"ip"`
					Alias    *string `json:"alias"`
					Port     int     `json:"port"`
					Assigned bool    `json:"assigned"`
				} `json:"attributes"`
			} `json:"data"`
			Meta struct {
				Pagination struct {
					TotalPages int `json:"total_pages"`
				} `json:"pagination"`
			} `json:"meta"`
		}
		if err := a.request(ctx, "GET", fmt.Sprintf("/api/application/nodes/%d/allocations?page=%d&per_page=100", node, page), nil, &r); err != nil {
			return nil, err
		}
		for _, entry := range r.Data {
			v := entry.Attributes
			host := v.IP
			if v.Alias != nil && *v.Alias != "" {
				host = *v.Alias
			}
			result = append(result, Allocation{ID: v.ID, Hostname: host, Port: v.Port, Assigned: v.Assigned})
		}
		if page >= r.Meta.Pagination.TotalPages {
			return result, nil
		}
	}
	return nil, ErrInvalid
}

// HTTPServerClient owns a distinct Client API credential and never reuses an Application key.
type HTTPServerClient struct {
	transport
	transferOrigins   []string
	allowTransferHTTP bool
}

func NewServerClient(base, key string, allowHTTP bool) (*HTTPServerClient, error) {
	t, err := newTransport(base, key, allowHTTP)
	if err != nil {
		return nil, err
	}
	return &HTTPServerClient{transport: t, allowTransferHTTP: allowHTTP}, nil
}

var _ ServerClient = (*HTTPServerClient)(nil)

func safeIdentifier(id string) bool {
	if len(id) < 8 || len(id) > 36 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}
func (a *HTTPServerClient) Resources(ctx context.Context, id string) (Usage, error) {
	if !safeIdentifier(id) {
		return Usage{}, ErrInvalid
	}
	var r struct {
		Attributes struct {
			State     string `json:"current_state"`
			Resources struct {
				CPU    float64 `json:"cpu_absolute"`
				RAM    int64   `json:"memory_bytes"`
				Disk   int64   `json:"disk_bytes"`
				Uptime int64   `json:"uptime"`
			} `json:"resources"`
		} `json:"attributes"`
	}
	err := a.request(ctx, "GET", "/api/client/servers/"+id+"/resources", nil, &r)
	v := r.Attributes
	return Usage{State: v.State, CPUPercent: v.Resources.CPU, MemoryBytes: v.Resources.RAM, DiskBytes: v.Resources.Disk, UptimeMillis: v.Resources.Uptime}, err
}
func (a *HTTPServerClient) Power(ctx context.Context, id string, signal PowerSignal) error {
	if !safeIdentifier(id) || (signal != PowerStart && signal != PowerStop && signal != PowerRestart && signal != PowerKill) {
		return ErrInvalid
	}
	return a.request(ctx, "POST", "/api/client/servers/"+id+"/power", map[string]string{"signal": string(signal)}, nil)
}
func (a *HTTPServerClient) SendCommand(ctx context.Context, id, command string) error {
	if !safeIdentifier(id) || command == "" || len(command) > 1024 || strings.ContainsAny(command, "\r\n") {
		return ErrInvalid
	}
	return a.request(ctx, "POST", "/api/client/servers/"+id+"/command", map[string]string{"command": command}, nil)
}

func (a *ApplicationHTTP) ReinstallServer(ctx context.Context, id int) error {
	return a.request(ctx, "POST", "/api/application/servers/"+strconv.Itoa(id)+"/reinstall", nil, nil)
}

type NodeInfo struct {
	ID, RAMMB, DiskMB, AllocatedRAMMB, AllocatedDiskMB int
	Name                                               string
	Maintenance                                        bool
}

func (a *ApplicationHTTP) GetNode(ctx context.Context, id int) (NodeInfo, error) {
	var r struct {
		Attributes struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			RAM         int    `json:"memory"`
			Disk        int    `json:"disk"`
			Maintenance bool   `json:"maintenance_mode"`
			Allocated   struct {
				RAM  int `json:"memory"`
				Disk int `json:"disk"`
			} `json:"allocated_resources"`
		} `json:"attributes"`
	}
	err := a.request(ctx, "GET", "/api/application/nodes/"+strconv.Itoa(id), nil, &r)
	if err != nil {
		return NodeInfo{}, err
	}
	v := r.Attributes
	if v.ID != id || id <= 0 {
		return NodeInfo{}, ErrInvalid
	}
	return NodeInfo{ID: id, Name: v.Name, RAMMB: v.RAM, DiskMB: v.Disk, AllocatedRAMMB: v.Allocated.RAM, AllocatedDiskMB: v.Allocated.Disk, Maintenance: v.Maintenance}, nil
}

type WebsocketCredentials struct{ Socket, Token string }
type ConsoleClient interface {
	WebsocketCredentials(context.Context, string) (WebsocketCredentials, error)
}

func (a *HTTPServerClient) WebsocketCredentials(ctx context.Context, id string) (WebsocketCredentials, error) {
	if !safeIdentifier(id) {
		return WebsocketCredentials{}, ErrInvalid
	}
	var r struct {
		Data struct {
			Socket string `json:"socket"`
			Token  string `json:"token"`
		} `json:"data"`
	}
	if err := a.request(ctx, "GET", "/api/client/servers/"+id+"/websocket", nil, &r); err != nil {
		return WebsocketCredentials{}, err
	}
	if r.Data.Socket == "" || r.Data.Token == "" || len(r.Data.Token) > 8192 {
		return WebsocketCredentials{}, ErrInvalid
	}
	return WebsocketCredentials{Socket: r.Data.Socket, Token: r.Data.Token}, nil
}
