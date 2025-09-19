package repository

import (
	"backend/internal/models"
	"fmt"

	"github.com/gocql/gocql"
)

type UserQRScanRepository struct {
	session *gocql.Session
}

func NewUserQRScanRepo(session *gocql.Session) *UserQRScanRepository {
	return &UserQRScanRepository{session: session}
}

func (r *UserQRScanRepository) CreateUserQRScan(user_qr_scan *models.UserQRScan) error {
	query := `INSERT INTO qr.user_qr_scans (user_id, qr_code_id, count) VALUES (?, ?, ?) IF NOT EXISTS`

	m := make(map[string]interface{})
	_, err := r.session.Query(query,
		user_qr_scan.UserId,
		user_qr_scan.QrCodeId,
		user_qr_scan.Count,
	).MapScanCAS(m)

	if err != nil {
		return err
	}
	return nil
}

func (r *UserQRScanRepository) GetUserQrScanByID(user_id, qr_code_id gocql.UUID) (*models.UserQRScan, error) {
	var userQRScan models.UserQRScan

	query := r.session.Query(`SELECT user_id, qr_code_id, count FROM qr.user_qr_scans WHERE user_id = ? AND qr_code_id = ? LIMIT 1`, user_id, qr_code_id).Consistency(gocql.LocalQuorum)

	err := query.Scan(
		&userQRScan.UserId,
		&userQRScan.QrCodeId,
		&userQRScan.Count,
	)

	if err != nil {
		return nil, err
	}

	return &userQRScan, nil
}

func (r *UserQRScanRepository) GetGlobalUsageCountByQRCodeId(qr_code_id gocql.UUID) (int, error) {
	var count int
	var totalCount int = 0

	iter := r.session.Query(`SELECT count FROM qr.user_qr_scans WHERE qr_code_id = ?`, qr_code_id).Iter()

	for iter.Scan(&count) {
		totalCount += count
	}

	if err := iter.Close(); err != nil {
		return 0, err
	}

	return totalCount, nil
}

func (r *UserQRScanRepository) UpdateCount(userId, qrCodeId gocql.UUID, newCount int) error {
	return r.session.Query(`UPDATE qr.user_qr_scans SET count = ? WHERE user_id = ? AND qr_code_id = ?`,
		newCount, userId, qrCodeId,
	).Exec()
}

func (r *UserQRScanRepository) DeleteUserQRCodeScansByUserId(userId gocql.UUID) error {
	query := `DELETE FROM qr.user_qr_scans WHERE user_id = ?`
	return r.session.Query(query, userId).Exec()
}

func (r *UserQRScanRepository) DeleteUserQRCodeScansByQRCodeId(qrCodeId gocql.UUID) error {
	iter := r.session.Query(`
		SELECT user_id, count
		FROM qr.user_qr_scans
		WHERE qr_code_id = ?
		ALLOW FILTERING
	`, qrCodeId).Iter()

	var userId gocql.UUID
	var count int

	for iter.Scan(&userId, &count) {
		if err := r.session.Query(`
			DELETE FROM qr.user_qr_scans
			WHERE user_id = ? AND qr_code_id = ?
		`, userId, qrCodeId).Exec(); err != nil {
			return fmt.Errorf("delete failed: %w", err)
		}
	}

	return nil
}
