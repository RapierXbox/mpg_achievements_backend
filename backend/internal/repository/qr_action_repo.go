package repository

import (
	"backend/internal/models"

	"github.com/gocql/gocql"
)

type QRActionRepository struct {
	session *gocql.Session
}

func NewQRActionRepo(session *gocql.Session) *QRActionRepository {
	return &QRActionRepository{session: session}
}

func (r *QRActionRepository) CreateQRAction(qr_action *models.QRAction) error {
	query := `INSERT INTO qr.qr_actions (id, action_json) VALUES (?, ?) IF NOT EXISTS`

	m := make(map[string]interface{})
	_, err := r.session.Query(query,
		qr_action.ID,
		qr_action.ActionJson,
	).MapScanCAS(m)

	if err != nil {
		return err
	}

	return nil
}

func (r *QRActionRepository) GetQRActionByID(id gocql.UUID) (*models.QRAction, error) {
	var account models.QRAction

	query := r.session.Query(`SELECT id, action_json FROM qr.qr_actions WHERE id = ? LIMIT 1 ALLOW FILTERING`, id).Consistency(gocql.LocalQuorum)

	err := query.Scan(
		&account.ID,
		&account.ActionJson,
	)

	if err == gocql.ErrNotFound {
		return nil, nil
	}
	return &account, err
}

func (r *QRActionRepository) DeleteQRAction(id gocql.UUID) error {
	query := `DELETE FROM qr.qr_actions WHERE id = ?`
	return r.session.Query(query, id).Exec()
}

func (r *QRActionRepository) GetAllQRActions(max_count int) ([]models.QRAction, error) {
	iter := r.session.Query("SELECT id, action_json FROM qr.qr_actions LIMIT ?", max_count).Iter()

	var entries []models.QRAction
	var id gocql.UUID
	var action_json string

	for iter.Scan(&id, &action_json) {
		entries = append(entries, models.QRAction{ID: id, ActionJson: action_json})
	}

	if err := iter.Close(); err != nil {
		return nil, err
	}

	return entries, nil
}
