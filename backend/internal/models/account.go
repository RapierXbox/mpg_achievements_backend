package models

import (
	"time"

	"github.com/google/uuid"
)

// account represents a user account
type Account struct {
	ID           uuid.UUID `json:"id"`         // unique user identifier
	Email        string    `json:"email"`      // email address
	PasswordHash string    `json:"-"`          // hashed password (never exposed)
	CreatedAt    time.Time `json:"created_at"` // account creation timestamp
}

// NewAccount creates a new account instance with initialized fields
func NewAccount(email, password string) *Account {
	return &Account{
		ID:        uuid.New(),       // generate unique ID
		Email:     email,            // set user email
		CreatedAt: time.Now().UTC(), // set creation time
	}
}
