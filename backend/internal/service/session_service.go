package service

import (
	"backend/internal/models"
	"backend/internal/repository"
	"backend/pkg/utils"

	"time"
)

// SessionService handles business logic for persistent sessions
type SessionService struct {
	repo     *repository.SessionRepository
	pepper   string
	tokenTTL time.Duration
}

// creates a new session service instance
func NewSessionService(repo *repository.SessionRepository, pepper string, tokenTTL time.Duration) *SessionService {
	return &SessionService{
		repo:     repo,
		pepper:   pepper,
		tokenTTL: tokenTTL,
	}
}

// CreatePermanentSession establishes a new device bound session
func (s *SessionService) CreatePermanentSession(userID, deviceID, token string) error {
	// secure hash token before storage
	hashedToken := utils.HashToken(token, s.pepper)

	// create session model
	session := &models.PermanentSession{
		UserID:    userID,
		DeviceID:  deviceID,
		TokenHash: hashedToken,
		CreatedAt: time.Now().UTC(),
		LastUsed:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(s.tokenTTL),
	}

	// persist to database
	return s.repo.CreateSession(session)
}

// ValidateSession checks if a session is valid and not expired
func (s *SessionService) ValidateSession(userID, deviceID, token string) (bool, error) {
	// retrieve session from database
	session, err := s.repo.GetSession(userID, deviceID)
	if err != nil || session == nil {
		return false, err
	}

	// check expiration
	if time.Now().UTC().After(session.ExpiresAt) {
		return false, nil
	}

	// verify token matches stored hash
	return utils.VerifyToken(token, s.pepper, session.TokenHash), nil
}

// CheckSession checks if a session is exist under the user and device id
func (s *SessionService) CheckSession(userID, deviceID string) (bool, error) {
	session, err := s.repo.GetSession(userID, deviceID)
	if err != nil {
		return false, err
	}
	if session == nil {
		return false, nil
	}
	return true, nil
}

// RotateSession updates a session with new credentials
func (s *SessionService) RotateSession(userID, deviceID, oldToken, newToken string) error {
	// validate existing token
	valid, err := s.ValidateSession(userID, deviceID, oldToken)
	if err != nil || !valid {
		return err
	}

	// create new secure hash
	newHash := utils.HashToken(newToken, s.pepper)

	// Update session in database
	return s.repo.RotateSessionToken(userID, deviceID, newHash)
}

// DeleteSession removes a session from the database
func (s *SessionService) DeleteSession(userID, deviceID string) error {
	return s.repo.DeleteSession(userID, deviceID)
}
