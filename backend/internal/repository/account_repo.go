package repository

import (
	"backend/internal/models"

	"errors"
	"fmt"

	"github.com/gocql/gocql"
	"github.com/google/uuid"
)

// AccountRepository handles database operations for user accounts
type AccountRepository struct {
	session *gocql.Session
}

// NewAccountRepo creates a new account repository instance
func NewAccountRepo(session *gocql.Session) *AccountRepository {
	return &AccountRepository{session: session}
}

// CreateAccount inserts a new user account into the database
func (r *AccountRepository) CreateAccount(account *models.Account) error {
	// use lightweight transaction to ensure email uniqueness
	query := `INSERT INTO auth.accounts (id, email, password_hash, created_at) VALUES (?, ?, ?, ?) IF NOT EXISTS`

	// convert uuid to bytes for proper scylladb marshaling
	idBytes, err := account.ID.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal UUID: %w", err)
	}

	m := make(map[string]interface{})
	applied, err := r.session.Query(query,
		idBytes,
		account.Email,
		account.PasswordHash,
		account.CreatedAt,
	).MapScanCAS(m) // check if applied

	if err != nil {
		return err
	}

	if !applied {
		return ErrEmailExists // custom error for duplicate email
	}
	return nil
}

// GetAccountByEmail retrieves an account by email address
func (r *AccountRepository) GetAccountByEmail(email string) (*models.Account, error) {
	var (
		idBytes []byte
		account models.Account
	)

	// consistancy level LocalQuorum for stronger consistency (doesnt matter on a single node)
	query := r.session.Query(`SELECT id, email, password_hash, created_at FROM auth.accounts WHERE email = ? LIMIT 1 ALLOW FILTERING`, email).Consistency(gocql.LocalQuorum)

	err := query.Scan(
		&idBytes,
		&account.Email,
		&account.PasswordHash,
		&account.CreatedAt,
	)

	if err == gocql.ErrNotFound {
		return nil, nil // account not found
	} else if err != nil {
		return nil, err
	}

	account.ID, err = uuid.FromBytes(idBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to convert uuid: %w", err)
	}
	return &account, err
}

// GetAccountByID retrieves an account by user ID
func (r *AccountRepository) GetAccountByID(id uuid.UUID) (*models.Account, error) {
	var account models.Account

	// ref. GetAccountByEmail
	query := r.session.Query(`SELECT id, email, password_hash, created_at FROM auth.accounts WHERE id = ? LIMIT 1 ALLOW FILTERING`, id).Consistency(gocql.LocalQuorum)

	err := query.Scan(
		&account.ID,
		&account.Email,
		&account.PasswordHash,
		&account.CreatedAt,
	)

	if err == gocql.ErrNotFound {
		return nil, nil
	}
	return &account, err
}

// UpdatePassword changes a user's password hash
func (r *AccountRepository) UpdatePassword(userID uuid.UUID, newHash string) error {
	return r.session.Query(`UPDATE auth.accounts SET password_hash = ? WHERE id = ?`,
		newHash, userID,
	).Exec()
}

// DeleteAccount removes a account from the database
func (r *AccountRepository) DeleteAccount(userID uuid.UUID) error {
	query := `DELETE FROM auth.accounts WHERE id = ?`
	return r.session.Query(query, userID).Exec()
}

// custom error for duplicate email
var ErrEmailExists = errors.New("account with email already exists")
