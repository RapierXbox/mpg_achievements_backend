package models

import "time"

// PermanentSession represents a long lived authentication session
type PermanentSession struct {
	UserID    string    `json:"user_id"`    // associated user ID
	DeviceID  string    `json:"device_id"`  // unique device identifier
	TokenHash string    `json:"token_hash"` // hashed refresh token
	CreatedAt time.Time `json:"created_at"` // creation timestamp
	LastUsed  time.Time `json:"last_used"`  // last access timestamp
	ExpiresAt time.Time `json:"expires_at"` // expiration timestamp
}
