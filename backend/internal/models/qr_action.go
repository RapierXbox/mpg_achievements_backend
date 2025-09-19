package models

import (
	"github.com/gocql/gocql"
)

type QRAction struct {
	ID         gocql.UUID `json:"id"`
	ActionJson string     `json:"action_json"`
}

func NewQRAction(action_json string) *QRAction {
	randomUUID, _ := gocql.RandomUUID() // ignoring error since it should never fail
	return &QRAction{
		ID:         randomUUID,
		ActionJson: action_json,
	}
}
