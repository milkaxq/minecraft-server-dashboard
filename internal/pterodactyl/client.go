// Package pterodactyl is the sole boundary to Panel APIs. It never runs processes.
package pterodactyl

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("pterodactyl resource not found")
var ErrConflict = errors.New("external id already exists with different configuration")
var ErrInvalid = errors.New("invalid pterodactyl request")

type Limits struct{ RAMMB, DiskMB, CPU, Backups, Databases int }
type UserRequest struct {
	ExternalID  uuid.UUID
	Email, Name string
}
type User struct {
	ID         int
	ExternalID uuid.UUID
}
type ServerRequest struct {
	ExternalID                          uuid.UUID
	UserID, NodeID, AllocationID, EggID int
	Name, Image, Startup                string
	Limits                              Limits
	Environment                         map[string]string
}
type Server struct {
	ID            int
	Identifier    string
	ExternalID    uuid.UUID
	Suspended     bool
	Installed     bool
	InstallFailed bool
	Request       ServerRequest
}

// ApplicationClient uses the privileged Application API. External IDs enable retry reconciliation.
type ApplicationClient interface {
	CreateUser(context.Context, UserRequest) (User, error)
	FindUserByExternalID(context.Context, uuid.UUID) (User, error)
	CreateServer(context.Context, ServerRequest) (Server, error)
	FindServerByExternalID(context.Context, uuid.UUID) (Server, error)
	GetServer(context.Context, int) (Server, error)
	DeleteServer(context.Context, int) error
	SuspendServer(context.Context, int) error
	UnsuspendServer(context.Context, int) error
	ReinstallServer(context.Context, int) error
}
type Usage struct {
	State                                string
	CPUPercent                           float64
	MemoryBytes, DiskBytes, UptimeMillis int64
}
type PowerSignal string

const (
	PowerStart   PowerSignal = "start"
	PowerStop    PowerSignal = "stop"
	PowerRestart PowerSignal = "restart"
	PowerKill    PowerSignal = "kill"
)

// ServerClient uses the separately authenticated Client API; identifiers are internal only.
// WebSocket credentials must be scoped to one server, never Application API credentials.
type ServerClient interface {
	Resources(context.Context, string) (Usage, error)
	Power(context.Context, string, PowerSignal) error
	SendCommand(context.Context, string, string) error
}

func ValidateRequest(r ServerRequest) error {
	if r.ExternalID == uuid.Nil || r.UserID <= 0 || r.NodeID <= 0 || r.AllocationID <= 0 || r.EggID <= 0 || r.Name == "" || r.Image == "" || r.Startup == "" || r.Limits.RAMMB <= 0 || r.Limits.DiskMB <= 0 || r.Limits.CPU <= 0 || r.Limits.Backups < 0 || r.Limits.Databases < 0 {
		return ErrInvalid
	}
	return nil
}
