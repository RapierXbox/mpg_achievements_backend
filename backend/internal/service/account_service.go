package service

import (
	"backend/internal/models"
	"backend/internal/repository"
	"backend/pkg/utils"

	"errors"
	"time"

	"github.com/google/uuid"
)

// AccountService handles business logic for user accounts
type AccountService struct {
	repo   *repository.AccountRepository
	pepper string
}

// NewAccountService creates a new account service instance
func NewAccountService(repo *repository.AccountRepository, pepper string) *AccountService {
	return &AccountService{
		repo:   repo,
		pepper: pepper,
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
	existing, err := s.repo.GetAccountByEmail(email)
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

	// create account object
	account := &models.Account{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: hash,
		CreatedAt:    time.Now().UTC(),
	}

	// persist to database
	if err := s.repo.CreateAccount(account); err != nil {
		return nil, err
	}

	return account, nil
}

// Authenticate verifies user credentials and returns account
func (s *AccountService) Authenticate(email, password string) (*models.Account, error) {
	// retrieve account
	account, err := s.repo.GetAccountByEmail(email)
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
func (s *AccountService) ChangePassword(userID uuid.UUID, oldPassword, newPassword string) error {
	if !utils.ValidatePassword(newPassword) {
		return errors.New("password must be at least 8 characters and contain at least one number, one uppercase letter, one lowercase letter")
	}

	// retrieve account
	account, err := s.repo.GetAccountByID(userID)
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
	return s.repo.UpdatePassword(userID, newHash)
}

// ChangePassword updates a user's password
func (s *AccountService) DeleteAccount(userID uuid.UUID) error {
	// retrieve account
	account, err := s.repo.GetAccountByID(userID)
	if err != nil || account == nil {
		return errors.New("account not found")
	}

	return s.repo.DeleteAccount(userID)
}
