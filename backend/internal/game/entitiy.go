package game

import (
	"backend/pkg/utils"
	"time"

	"github.com/gocql/gocql"
)

type Entity struct {
	ID         gocql.UUID
	ParentID   gocql.UUID
	Type       uint16
	Position   utils.Vector3
	Rotation   utils.Vector3
	CustomData []byte

	LastUpdated time.Time
}

func NewEntity(id, parentID gocql.UUID, entityType uint16, position utils.Vector3, rotation utils.Vector3, customData []byte) Entity {
	return Entity{
		ID:         id,
		ParentID:   parentID,
		Type:       entityType,
		Position:   position,
		Rotation:   rotation,
		CustomData: customData,

		LastUpdated: time.Now(),
	}
}
