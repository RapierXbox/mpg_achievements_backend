package service

import (
	"backend/internal/models"
	"backend/internal/repository"
	"backend/pkg/utils"

	"errors"
	"time"

	"github.com/gocql/gocql"
)

// AccountService handles business logic for user accounts
type AccountService struct {
	account_repo *repository.AccountRepository
	session_repo *repository.SessionRepository
	pepper       string
}

// NewAccountService creates a new account service instance
func NewAccountService(account_repo *repository.AccountRepository, session_repo *repository.SessionRepository, pepper string) *AccountService {
	return &AccountService{
		account_repo: account_repo,
		session_repo: session_repo,
		pepper:       pepper,
	}
}

// RegisterAccount creates a new user account with proper validation
func (s *AccountService) RegisterAccount(email, password string) (*models.Account, error) {
	// validate input
	if !utils.ValidateEmail(email) {
		return nil, errors.New("invalid email format")
	}
	if !utils.ValidatePassword(password) {
		return nil, errors.New("password must be at least 8 characters and contain at least one number, one uppercase letter, one lowercase letter")
	}

	// check if account exists
	existing, err := s.account_repo.GetAccountByEmail(email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, repository.ErrEmailExists
	}

	// hash password with server side pepper
	hash, err := utils.HashPassword(password, s.pepper)
	if err != nil {
		return nil, err
	}

	randomUUID, _ := gocql.RandomUUID() // ignoring error since it should never fail

	// create account object
	account := &models.Account{
		ID:           randomUUID,
		Email:        email,
		PasswordHash: hash,
		CreatedAt:    time.Now().UTC(),
	}

	// persist to database
	if err := s.account_repo.CreateAccount(account); err != nil {
		return nil, err
	}

	return account, nil
}

// Authenticate verifies user credentials and returns account
func (s *AccountService) Authenticate(email, password string) (*models.Account, error) {
	// retrieve account
	account, err := s.account_repo.GetAccountByEmail(email)
	if err != nil || account == nil {
		return nil, errors.New("invalid credentials - " + err.Error())
	}

	// verify password
	if !utils.CheckPasswordHash(password, s.pepper, account.PasswordHash) {
		return nil, errors.New("invalid credentials")
	}

	return account, nil
}

// ChangePassword updates a user's password
func (s *AccountService) ChangePassword(userID gocql.UUID, oldPassword, newPassword string) error {
	if !utils.ValidatePassword(newPassword) {
		return errors.New("password must be at least 8 characters and contain at least one number, one uppercase letter, one lowercase letter")
	}

	// retrieve account
	account, err := s.account_repo.GetAccountByID(userID)
	if err != nil || account == nil {
		return errors.New("account not found")
	}

	// verify current password
	if !utils.CheckPasswordHash(oldPassword, s.pepper, account.PasswordHash) {
		return errors.New("invalid current password")
	}

	// generate new hash
	newHash, err := utils.HashPassword(newPassword, s.pepper)
	if err != nil {
		return err
	}

	// update database
	return s.account_repo.UpdatePassword(userID, newHash)
}

// ChangePassword updates a user's password
func (s *AccountService) DeleteAccount(userID gocql.UUID) error {
	// retrieve account
	account, err := s.account_repo.GetAccountByID(userID)
	if err != nil || account == nil {
		return errors.New("account not found")
	}

	err = s.account_repo.DeleteAccount(userID)
	if err != nil {
		return errors.New("failed to delete account - " + err.Error())
	}

	err = s.session_repo.DeleteAllSessionsForUser(userID)
	if err != nil {
		return errors.New("failed to delete associated sessions - " + err.Error())
	}

	return nil
}
