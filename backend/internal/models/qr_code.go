package models

import (
	"time"

	"github.com/gocql/gocql"
)

type QRCodeUsageType int

const (
	PerAccount QRCodeUsageType = iota // the cound decreases only for the user who used it
	Global                            // the count decreases for all users
)

type QRCode struct {
	ID         gocql.UUID      `json:"id"`
	ActionId   gocql.UUID      `json:"action_id"`
	QrCodeType QRCodeUsageType `json:"qr_type"`
	MaxUsages  int             `json:"max_uses"` // maximum number of uses (0 for unlimited)
	ExpiresAt  time.Time       `json:"expires_at"`
}

func NewQRCode(action_id gocql.UUID, qr_code_type QRCodeUsageType, max_usages int, expiration_timestamp time.Time) *QRCode {
	randomUUID, _ := gocql.RandomUUID() // ignoring error since it should never fail
	return &QRCode{
		ID:         randomUUID,
		ActionId:   action_id,
		QrCodeType: qr_code_type,
		MaxUsages:  max_usages,
		ExpiresAt:  expiration_timestamp,
	}
}
