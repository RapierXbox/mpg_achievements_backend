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

func NewSessionRepo(session *gocql.Session) *SessionRepository {
	return &SessionRepository{session: session}
}

func (r *SessionRepository) CreateSession(session *models.PermanentSession) error {
	// use parameterized query to prevent sql injection
	query := `INSERT INTO auth.permanent_sessions 
		(user_id, device_id, token_hash, created_at, last_used, expires_at) 
		VALUES (?, ?, ?, ?, ?, ?)`

	return r.session.Query(query,
		session.UserID,
		session.DeviceID,
		session.TokenHash,
		session.CreatedAt,
		session.LastUsed,
		session.ExpiresAt,
	).Exec()
}

func (r *SessionRepository) GetSession(userID, deviceID gocql.UUID) (*models.PermanentSession, error) {
	session := &models.PermanentSession{
		UserID:   userID,
		DeviceID: deviceID,
	}

	query := `SELECT token_hash, expires_at 
		FROM auth.permanent_sessions 
		WHERE user_id = ? AND device_id = ? 
		LIMIT 1`

	err := r.session.Query(query, userID, deviceID).Scan(
		&session.TokenHash,
		&session.ExpiresAt,
	)

	if err == gocql.ErrNotFound {
		return nil, nil
	}

	return session, err
}

func (r *SessionRepository) RotateSessionToken(userID, deviceID gocql.UUID, newTokenHash string) error {
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

func (r *SessionRepository) DeleteSession(userID, deviceID gocql.UUID) error {
	query := `DELETE FROM auth.permanent_sessions 
		WHERE user_id = ? AND device_id = ?`
	return r.session.Query(query, userID, deviceID).Exec()
}

func (r *SessionRepository) DeleteAllSessionsForUser(userID gocql.UUID) error {
	query := `DELETE FROM auth.permanent_sessions 
		WHERE user_id = ?`
	return r.session.Query(query, userID).Exec()
}
