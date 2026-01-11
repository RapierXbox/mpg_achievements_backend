package game

import (
	"backend/pkg/utils"
	"sync/atomic"
	"time"

	"github.com/gocql/gocql"
)

type Entity struct {
	ID         gocql.UUID
	ParentID   gocql.UUID
	Type       uint16
	Position   atomic.Value
	Rotation   atomic.Value
	CustomData []byte

	LastUpdatedNanos atomic.Int64
}

func NewEntity(id, parentID gocql.UUID, entityType uint16, position, rotation utils.Vector3, customData []byte) *Entity {
	entity := &Entity{
		ID:         id,
		ParentID:   parentID,
		Type:       entityType,
		CustomData: customData,
	}
	entity.Position.Store(position)
	entity.Rotation.Store(rotation)
	entity.LastUpdatedNanos.Store(time.Now().UnixNano())
	return entity
}

func (e *Entity) UpdatePositon(position, rotation utils.Vector3) {
	e.Position.Store(position)
	e.Rotation.Store(rotation)
	e.LastUpdatedNanos.Store(time.Now().UnixNano())
}
