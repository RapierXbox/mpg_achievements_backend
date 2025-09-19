package models

import (
	"time"

	"github.com/gocql/gocql"
)

// account represents a user account
type Account struct {
	ID           gocql.UUID `json:"id"`         // unique user identifier
	Email        string     `json:"email"`      // email address
	PasswordHash string     `json:"-"`          // hashed password (never exposed)
	CreatedAt    time.Time  `json:"created_at"` // account creation timestamp
	Admin        bool       `json:"admin"`      // admin privileges
}

// NewAccount creates a new account instance with initialized fields
func NewAccount(email, password string) *Account {
	randomUUID, _ := gocql.RandomUUID() // ignoring error since it should never fail

	return &Account{
		ID:        randomUUID,       // generate unique ID
		Email:     email,            // set user email
		CreatedAt: time.Now().UTC(), // set creation time
		Admin:     false,            // default to non-admin
	}
}
