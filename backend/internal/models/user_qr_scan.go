package models

import (
	"github.com/gocql/gocql"
)

type UserQRScan struct {
	UserID   gocql.UUID `json:"user_id"`
	QrCodeID gocql.UUID `json:"qr_code_id"`
	Count    int        `json:"count"`
}

func NewUserQRScan(user_id, qr_code_id gocql.UUID) *UserQRScan {
	return &UserQRScan{
		UserID:   user_id,
		QrCodeID: qr_code_id,
		Count:    0,
	}
}
