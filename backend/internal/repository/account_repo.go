package repository

import (
	"backend/internal/models"

	"errors"

	"github.com/gocql/gocql"
)

// AccountRepository handles database operations for user accounts
type AccountRepository struct {
	session *gocql.Session
}

func NewAccountRepo(session *gocql.Session) *AccountRepository {
	return &AccountRepository{session: session}
}

func (r *AccountRepository) CreateAccount(account *models.Account) error {
	// use lightweight transaction to ensure email uniqueness
	query := `INSERT INTO auth.accounts (id, email, password_hash, created_at, admin) VALUES (?, ?, ?, ?, ?) IF NOT EXISTS`

	m := make(map[string]interface{})
	applied, err := r.session.Query(query,
		account.ID,
		account.Email,
		account.PasswordHash,
		account.CreatedAt,
		account.Admin,
	).MapScanCAS(m) // check if applied

	if err != nil {
		return err
	}

	if !applied {
		return ErrEmailExists // custom error for duplicate email
	}
	return nil
}

func (r *AccountRepository) GetAccountByEmail(email string) (*models.Account, error) {
	var account models.Account

	// consistancy level LocalQuorum for stronger consistency (doesnt matter on a single node)
	query := r.session.Query(`SELECT id, email, password_hash, created_at, admin FROM auth.accounts WHERE email = ? LIMIT 1`, email).Consistency(gocql.LocalQuorum)

	err := query.Scan(
		&account.ID,
		&account.Email,
		&account.PasswordHash,
		&account.CreatedAt,
		&account.Admin,
	)

	if err == gocql.ErrNotFound {
		return nil, nil // account not found
	} else if err != nil {
		return nil, err
	}

	return &account, err
}

func (r *AccountRepository) GetAccountByID(id gocql.UUID) (*models.Account, error) {
	var account models.Account

	// ref. GetAccountByEmail
	query := r.session.Query(`SELECT id, email, password_hash, created_at, admin FROM auth.accounts WHERE id = ? LIMIT 1`, id).Consistency(gocql.LocalQuorum)

	err := query.Scan(
		&account.ID,
		&account.Email,
		&account.PasswordHash,
		&account.CreatedAt,
		&account.Admin,
	)

	if err == gocql.ErrNotFound {
		return nil, nil
	}
	return &account, err
}

func (r *AccountRepository) UpdatePassword(userID gocql.UUID, newHash string) error {
	return r.session.Query(`UPDATE auth.accounts SET password_hash = ? WHERE id = ?`,
		newHash, userID,
	).Exec()
}

func (r *AccountRepository) DeleteAccount(userID gocql.UUID) error {
	query := `DELETE FROM auth.accounts WHERE id = ?`
	return r.session.Query(query, userID).Exec()
}

// custom error for duplicate email
var ErrEmailExists = errors.New("account with email already exists")
