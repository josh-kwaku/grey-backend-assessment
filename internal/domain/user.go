package domain

import (
	"time"

	"github.com/google/uuid"
)

type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusSuspended UserStatus = "suspended"
	UserStatusClosed    UserStatus = "closed"
)

type User struct {
	ID           uuid.UUID
	Email        string
	Name         string
	PasswordHash string
	UniqueName   *string
	Status       UserStatus
	CreatedAt    time.Time
}
