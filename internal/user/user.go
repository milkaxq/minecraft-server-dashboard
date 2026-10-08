package user

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	IsAdmin    bool       `json:"is_admin"`
	ID         uuid.UUID  `json:"id"`
	Email      string     `json:"email"`
	Name       string     `json:"name"`
	VerifiedAt *time.Time `json:"verified_at"`
	CreatedAt  time.Time  `json:"created_at"`
}
