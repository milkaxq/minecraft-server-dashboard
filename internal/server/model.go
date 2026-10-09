package server

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

type State string

const (
	PendingPayment   State = "PENDING_PAYMENT"
	PaymentConfirmed State = "PAYMENT_CONFIRMED"
	Provisioning     State = "PROVISIONING"
	Running          State = "RUNNING"
	Suspended        State = "SUSPENDED"
	Failed           State = "FAILED"
	Cancelled        State = "CANCELLED"
)

func CanTransition(from, to State) bool {
	switch from {
	case PendingPayment:
		return to == PaymentConfirmed || to == Cancelled
	case PaymentConfirmed:
		return to == Provisioning || to == Cancelled
	case Provisioning:
		return to == Running || to == Failed
	case Running:
		return to == Suspended || to == Cancelled
	case Suspended:
		return to == Running || to == Cancelled
	case Failed:
		return to == Provisioning || to == Cancelled
	}
	return false
}

type Role string

const (
	Owner     Role = "OWNER"
	Admin     Role = "ADMIN"
	Moderator Role = "MODERATOR"
)

type Permission string

const (
	View       Permission = "view"
	Console    Permission = "console"
	Start      Permission = "start"
	Restart    Permission = "restart"
	Stop       Permission = "stop"
	Kill       Permission = "kill"
	Files      Permission = "files"
	Settings   Permission = "settings"
	Plugins    Permission = "plugins"
	Worlds     Permission = "worlds"
	Backups    Permission = "backups"
	Billing    Permission = "billing"
	Members    Permission = "members"
	Delete     Permission = "delete"
	AcceptEULA Permission = "accept_eula"
)

func Allowed(role Role, permission Permission) bool {
	switch permission {
	case View, Console, Start, Restart:
		return role == Owner || role == Admin || role == Moderator
	case Stop, Kill, Files, Settings, Plugins, Worlds, Backups:
		return role == Owner || role == Admin
	case Billing, Members, Delete, AcceptEULA:
		return role == Owner
	default:
		return false
	}
}

var ErrNotFound = errors.New("server not found")
var ErrInvalid = errors.New("invalid server input")
var versionPattern = regexp.MustCompile(`^[0-9]{1,2}\.[0-9]{1,2}(\.[0-9]{1,2})?$`)

type CreateInput struct {
	PlanID           uuid.UUID `json:"plan_id"`
	Name             string    `json:"name"`
	MinecraftVersion string    `json:"minecraft_version"`
	ServerType       string    `json:"server_type"`
	LoaderVersion    string    `json:"loader_version,omitempty"`
}

func (i *CreateInput) Validate() error {
	i.Name = strings.TrimSpace(i.Name)
	if i.PlanID == uuid.Nil || len(i.Name) == 0 || len(i.Name) > 80 || !versionPattern.MatchString(i.MinecraftVersion) || !ValidSoftware(i.ServerType) {
		return ErrInvalid
	}
	for _, c := range i.Name {
		if c < 32 || c == 127 {
			return ErrInvalid
		}
	}
	return nil
}

type Server struct {
	ID               uuid.UUID `json:"id"`
	OwnerID          uuid.UUID `json:"owner_id"`
	PlanID           uuid.UUID `json:"plan_id"`
	Name             string    `json:"name"`
	Status           State     `json:"status"`
	MinecraftVersion string    `json:"minecraft_version"`
	ServerType       string    `json:"server_type"`
	LoaderVersion    string    `json:"loader_version,omitempty"`
	Hostname         *string   `json:"hostname"`
	Port             *int      `json:"port"`
	RAMMB            int       `json:"ram_mb"`
	DiskMB           int       `json:"disk_mb"`
	CPULimit         int       `json:"cpu_limit"`
	BackupLimit      int       `json:"backup_limit"`
	DatabaseLimit    int       `json:"database_limit"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
