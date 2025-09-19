package service

import (
	"backend/internal/models"
	"backend/internal/repository"
	"fmt"
	"time"

	"errors"

	"github.com/gocql/gocql"
)

type QRService struct {
	action_repo *repository.QRActionRepository
	code_repo   *repository.QRCodeRepository
	scan_repo   *repository.UserQRScanRepository
}

func NewQRService(action_repo *repository.QRActionRepository, code_repo *repository.QRCodeRepository, scan_repo *repository.UserQRScanRepository) *QRService {
	return &QRService{
		action_repo: action_repo,
		code_repo:   code_repo,
		scan_repo:   scan_repo,
	}
}

func (s *QRService) AddQRCode(action_id gocql.UUID, qr_code_type models.QRCodeUsageType, max_usages int, expires_at time.Time) (*models.QRCode, error) {
	// check if action exists
	action, err := s.action_repo.GetQRActionByID(action_id)
	if err != nil {
		return nil, errors.New("failed to get qr action - " + err.Error())
	}
	if action == nil {
		return nil, errors.New("this action code does not exist")
	}

	randomUUID, _ := gocql.RandomUUID() // ignoring error since it should never fail

	qr_code := &models.QRCode{
		ID:         randomUUID,
		ActionId:   action_id,
		QrCodeType: qr_code_type,
		MaxUsages:  max_usages,
		ExpiresAt:  expires_at,
	}

	err = s.code_repo.CreateQRCode(qr_code)
	if err != nil {
		return nil, err
	}

	return qr_code, nil

}

func (s *QRService) GetActionJsonFromQRCodeId(qr_code_id, user_id gocql.UUID) (string, error) {
	// get qr code
	qr_code, err := s.code_repo.GetQRCodeByID(qr_code_id)
	if err != nil {
		return "", errors.New("failed to get qr code - " + err.Error())
	}
	if qr_code == nil {
		return "", errors.New("this qr code does not exist")
	}

	if qr_code.ExpiresAt.Before(time.Now().UTC()) {
		s.code_repo.DeleteQRCode(qr_code.ID) // clean up expired qr code
		return "", errors.New("this qr code is expired")
	}

	qr_scan, err := s.scan_repo.GetUserQrScanByID(user_id, qr_code_id)
	if err == gocql.ErrNotFound {
		qr_scan = &models.UserQRScan{
			UserId:   user_id,
			QrCodeId: qr_code.ID,
			Count:    0,
		}
		err = s.scan_repo.CreateUserQRScan(qr_scan)
		if err != nil {
			return "", errors.New("failed to create user qr scan - " + err.Error())
		}
	} else if err != nil {
		return "", errors.New("failed to get user qr scan - " + err.Error())
	}

	// check usage limits
	switch qr_code.QrCodeType {
	case models.PerAccount:
		if qr_code.MaxUsages > 0 && qr_scan.Count >= qr_code.MaxUsages {
			return "", fmt.Errorf("this qr code has reached its maximum number of uses for this account (type: %d, max usages: %d, usages: %d)", qr_code.QrCodeType, qr_code.MaxUsages, qr_scan.Count)
		}
	case models.Global:
		global_usage, err := s.scan_repo.GetGlobalUsageCountByQRCodeId(qr_code.ID)
		if err != nil {
			return "", errors.New("failed to get global usage count - " + err.Error())
		}
		if qr_code.MaxUsages > 0 && (global_usage >= qr_code.MaxUsages || qr_scan.Count > 1) {
			return "", errors.New("this qr code has reached its maximum number of uses")
		}
	}

	qr_action, err := s.action_repo.GetQRActionByID(qr_code.ActionId)
	if err != nil || qr_action == nil {
		return "", errors.New("this action doesnt exist - " + err.Error())
	}

	err = s.scan_repo.UpdateCount(user_id, qr_code_id, qr_scan.Count+1)

	if err != nil {
		return "", errors.New("failed to update usage count - " + err.Error())
	}
	return qr_action.ActionJson, nil

}

func (s *QRService) DeleteQRCode(id gocql.UUID) error {
	qr_code, err := s.code_repo.GetQRCodeByID(id)
	if err != nil {
		return errors.New("failed to get qr code - " + err.Error())
	}
	if qr_code == nil {
		return errors.New("this qr code does not exist")
	}

	err = s.scan_repo.DeleteUserQRCodeScansByQRCodeId(id)
	if err != nil {
		return errors.New("failed to delete associated user qr scans - " + err.Error())
	}

	return s.code_repo.DeleteQRCode(id)
}

func (s *QRService) GetAllQRCodes(max_count int) ([]models.QRCode, error) {
	return s.code_repo.GetAllQRCodes(max_count)
}

// -------------------------------------- QR ACTIONS -----------------------------------------------
func (s *QRService) AddQRAction(action_json string) (*models.QRAction, error) {
	randomUUID, _ := gocql.RandomUUID() // ignoring error since it should never fail
	qr_action := &models.QRAction{
		ID:         randomUUID,
		ActionJson: action_json,
	}

	if err := s.action_repo.CreateQRAction(qr_action); err != nil {
		return nil, err
	}

	return qr_action, nil
}

func (s *QRService) GetQRActionById(id gocql.UUID) (*models.QRAction, error) {
	qr_action, err := s.action_repo.GetQRActionByID(id)
	if err != nil || qr_action == nil {
		return nil, errors.New("this action doesnt exist - " + err.Error())
	}

	return qr_action, nil
}

func (s *QRService) DeleteQRAction(id gocql.UUID) error {
	qr_action, err := s.action_repo.GetQRActionByID(id)
	if err != nil || qr_action == nil {
		return errors.New("qr action not found")
	}

	return s.action_repo.DeleteQRAction(id)
}

func (s *QRService) GetAllQRActions(max_count int) ([]models.QRAction, error) {
	return s.action_repo.GetAllQRActions(max_count)
}
