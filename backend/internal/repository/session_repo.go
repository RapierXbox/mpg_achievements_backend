package repository

import (
	"backend/internal/models"

	"time"

	"github.com/gocql/gocql"
)

// SessionRepository handles database operations for persistent sessions
type SessionRepository struct {
	session *gocql.Session
}

// creates a new session repository instance
func NewSessionRepo(session *gocql.Session) *SessionRepository {
	return &SessionRepository{session: session}
}

// CreateSession stores a new permanent session in the database
func (r *SessionRepository) CreateSession(session *models.PermanentSession) error {
	// use parameterized query to prevent sql injection
	query := `INSERT INTO auth.permanent_sessions 
		(user_id, device_id, token_hash, created_at, last_used, expires_at) 
		VALUES (?, ?, ?, ?, ?, ?)`

	// execute insert with session data
	return r.session.Query(query,
		session.UserID,
		session.DeviceID,
		session.TokenHash,
		session.CreatedAt,
		session.LastUsed,
		session.ExpiresAt,
	).Exec()
}

// GetSession retrieves a session by user id and device id
func (r *SessionRepository) GetSession(userID, deviceID string) (*models.PermanentSession, error) {
	session := &models.PermanentSession{
		UserID:   userID,
		DeviceID: deviceID,
	}

	// select session data
	query := `SELECT token_hash, expires_at 
		FROM auth.permanent_sessions 
		WHERE user_id = ? AND device_id = ? 
		LIMIT 1 ALLOW FILTERING`

	// scan results into session object
	err := r.session.Query(query, userID, deviceID).Scan(
		&session.TokenHash,
		&session.ExpiresAt,
	)

	if err == gocql.ErrNotFound {
		return nil, nil // session not found
	}

	return session, err
}

// RotateSessionToken updates a session with a new token hash
func (r *SessionRepository) RotateSessionToken(userID, deviceID, newTokenHash string) error {
	// update token hash and usage timestamps
	query := `UPDATE auth.permanent_sessions SET 
		token_hash = ?,
		last_used = ? 
		WHERE user_id = ? AND device_id = ?`

	return r.session.Query(query,
		newTokenHash,
		time.Now().UTC(),
		userID,
		deviceID,
	).Exec()
}

// DeleteSession removes a session from the database
func (r *SessionRepository) DeleteSession(userID, deviceID string) error {
	query := `DELETE FROM auth.permanent_sessions 
		WHERE user_id = ? AND device_id = ?`
	return r.session.Query(query, userID, deviceID).Exec()
}
