package models

import (
	"github.com/gocql/gocql"
)

type UserQRScan struct {
	UserId   gocql.UUID `json:"user_id"`
	QrCodeId gocql.UUID `json:"qr_code_id"`
	Count    int        `json:"count"`
}

func NewUserQRScan(user_id, qr_code_id gocql.UUID) *UserQRScan {
	return &UserQRScan{
		UserId:   user_id,
		QrCodeId: qr_code_id,
		Count:    0,
	}
}
