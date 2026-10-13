package pterodactyl

import (
	"context"
	"reflect"
	"sync"

	"github.com/google/uuid"
)

// Mock is concurrency safe, process-local and deliberately non-persistent. It starts no containers.
type Mock struct {
	mu                   sync.Mutex
	nextUser, nextServer int
	users                map[uuid.UUID]User
	userRequests         map[uuid.UUID]UserRequest
	servers              map[int]Server
	external             map[uuid.UUID]int
	states               map[string]string
	extraAllocations     map[int][]int
}

func NewMock() *Mock {
	return &Mock{extraAllocations: map[int][]int{}, users: map[uuid.UUID]User{}, userRequests: map[uuid.UUID]UserRequest{}, servers: map[int]Server{}, external: map[uuid.UUID]int{}, states: map[string]string{}}
}

var _ ApplicationClient = (*Mock)(nil)
var _ ServerClient = (*Mock)(nil)

func (m *Mock) CreateUser(ctx context.Context, r UserRequest) (User, error) {
	if err := ctx.Err(); err != nil {
		return User{}, err
	}
	if r.ExternalID == uuid.Nil || r.Email == "" {
		return User{}, ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.users[r.ExternalID]; ok {
		if m.userRequests[r.ExternalID] != r {
			return User{}, ErrConflict
		}
		return u, nil
	}
	m.nextUser++
	u := User{ID: m.nextUser, ExternalID: r.ExternalID}
	m.users[r.ExternalID] = u
	m.userRequests[r.ExternalID] = r
	return u, nil
}
func clone(s Server) Server {
	var env map[string]string
	if s.Request.Environment != nil {
		env = make(map[string]string, len(s.Request.Environment))
	}
	for k, v := range s.Request.Environment {
		env[k] = v
	}
	s.Request.Environment = env
	return s
}
func (m *Mock) CreateServer(ctx context.Context, r ServerRequest) (Server, error) {
	if err := ctx.Err(); err != nil {
		return Server{}, err
	}
	if r.ExternalID == uuid.Nil || r.UserID <= 0 || r.NodeID <= 0 || r.EggID <= 0 || r.AllocationID <= 0 || r.Name == "" || r.Image == "" || r.Startup == "" || r.Limits.RAMMB <= 0 || r.Limits.DiskMB <= 0 || r.Limits.CPU <= 0 || r.Limits.Backups < 0 || r.Limits.Databases < 0 {
		return Server{}, ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.external[r.ExternalID]; ok {
		s := m.servers[id]
		if !reflect.DeepEqual(s.Request, r) {
			return Server{}, ErrConflict
		}
		return clone(s), nil
	}
	found := false
	for _, u := range m.users {
		if u.ID == r.UserID {
			found = true
		}
	}
	if !found {
		return Server{}, ErrNotFound
	}
	m.nextServer++
	s := clone(Server{ID: m.nextServer, Identifier: uuid.NewString()[:8], ExternalID: r.ExternalID, Installed: true, Request: r})
	m.servers[s.ID] = s
	m.external[s.ExternalID] = s.ID
	m.states[s.Identifier] = "offline"
	return clone(s), nil
}
func (m *Mock) GetServer(ctx context.Context, id int) (Server, error) {
	if err := ctx.Err(); err != nil {
		return Server{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.servers[id]
	if !ok {
		return Server{}, ErrNotFound
	}
	return clone(s), nil
}
func (m *Mock) FindServerByExternalID(ctx context.Context, id uuid.UUID) (Server, error) {
	if err := ctx.Err(); err != nil {
		return Server{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.external[id]
	if !ok {
		return Server{}, ErrNotFound
	}
	return clone(m.servers[n]), nil
}
func (m *Mock) DeleteServer(ctx context.Context, id int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.servers[id]; ok {
		delete(m.external, s.ExternalID)
		delete(m.states, s.Identifier)
		delete(m.servers, id)
		delete(m.extraAllocations, id)
	}
	return nil
}
func (m *Mock) suspend(ctx context.Context, id int, value bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.servers[id]
	if !ok {
		return ErrNotFound
	}
	s.Suspended = value
	m.servers[id] = s
	if value {
		m.states[s.Identifier] = "offline"
	}
	return nil
}
func (m *Mock) SuspendServer(ctx context.Context, id int) error   { return m.suspend(ctx, id, true) }
func (m *Mock) UnsuspendServer(ctx context.Context, id int) error { return m.suspend(ctx, id, false) }
func (m *Mock) Resources(ctx context.Context, id string) (Usage, error) {
	if err := ctx.Err(); err != nil {
		return Usage{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.states[id]
	if !ok {
		return Usage{}, ErrNotFound
	}
	return Usage{State: state}, nil
}
func (m *Mock) Power(ctx context.Context, id string, signal PowerSignal) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.states[id]; !ok {
		return ErrNotFound
	}
	for _, s := range m.servers {
		if s.Identifier == id && s.Suspended {
			return ErrConflict
		}
	}
	switch signal {
	case PowerStart, PowerRestart:
		m.states[id] = "running"
	case PowerStop, PowerKill:
		m.states[id] = "offline"
	default:
		return ErrInvalid
	}
	return nil
}
func (m *Mock) SendCommand(ctx context.Context, id, command string) error {
	if command == "" || len(command) > 1024 {
		return ErrInvalid
	}
	u, err := m.Resources(ctx, id)
	if err != nil {
		return err
	}
	if u.State != "running" {
		return ErrConflict
	}
	return nil
}

func (m *Mock) FindUserByExternalID(ctx context.Context, id uuid.UUID) (User, error) {
	if err := ctx.Err(); err != nil {
		return User{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

func (m *Mock) ReinstallServer(ctx context.Context, id int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.servers[id]
	if !ok {
		return ErrNotFound
	}
	s.InstallFailed = false
	s.Installed = true
	m.servers[id] = s
	return nil
}
