package repository

import (
	"backend/internal/models"
	"time"

	"github.com/gocql/gocql"
)

type QRCodeRepository struct {
	session *gocql.Session
}

func NewQRCodeRepo(session *gocql.Session) *QRCodeRepository {
	return &QRCodeRepository{session: session}
}

func (r *QRCodeRepository) CreateQRCode(qr_code *models.QRCode) error {
	query := `INSERT INTO qr.qr_codes (id, action_id, qr_code_type, max_usages, expires_at) VALUES (?, ?, ?, ?, ?) IF NOT EXISTS`

	m := make(map[string]interface{})
	_, err := r.session.Query(query,
		qr_code.ID,
		qr_code.ActionId,
		qr_code.QrCodeType,
		qr_code.MaxUsages,
		qr_code.ExpiresAt,
	).MapScanCAS(m)

	if err != nil {
		return err
	}

	return nil
}

func (r *QRCodeRepository) GetQRCodeByID(id gocql.UUID) (*models.QRCode, error) {
	var code models.QRCode

	query := r.session.Query(`SELECT id, action_id, qr_code_type, max_usages, expires_at FROM qr.qr_codes WHERE id = ? LIMIT 1`, id).Consistency(gocql.LocalQuorum)

	err := query.Scan(
		&code.ID,
		&code.ActionId,
		&code.QrCodeType,
		&code.MaxUsages,
		&code.ExpiresAt,
	)

	if err == gocql.ErrNotFound {
		return nil, nil
	}
	return &code, err
}

func (r *QRCodeRepository) DeleteQRCode(id gocql.UUID) error {
	query := `DELETE FROM qr.qr_codes WHERE id = ?`
	return r.session.Query(query, id).Exec()
}

func (r *QRCodeRepository) GetAllQRCodes(max_count int) ([]models.QRCode, error) {
	iter := r.session.Query("SELECT id, action_id, qr_code_type, max_usages, expires_at FROM qr.qr_codes LIMIT ?", max_count).Iter()

	var entries []models.QRCode
	var id gocql.UUID
	var action_id gocql.UUID
	var qr_code_type models.QRCodeUsageType
	var max_usages int
	var expires_at time.Time

	for iter.Scan(&id, &action_id, &qr_code_type, &max_usages, &expires_at) {
		entries = append(entries, models.QRCode{ID: id, ActionId: action_id, QrCodeType: qr_code_type, MaxUsages: max_usages, ExpiresAt: expires_at})
	}

	if err := iter.Close(); err != nil {
		return nil, err
	}

	return entries, nil
}
