package game

import (
	"backend/pkg/network"
	"context"
	"encoding/binary"
	"errors"
	"math/rand"
	"sync"

	"github.com/gocql/gocql"
)

var (
	ErrRoomNotFound   = errors.New("room not found")
	ErrRoomFull       = errors.New("room is full")
	ErrPlayerNotFound = errors.New("player not found")
	ErrAlreadyInRoom  = errors.New("player already in a room")
)

type Manager struct {
	rooms         map[uint32]*Room
	sessionToRoom map[gocql.UUID]*uint32
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc

	maxRoomCapacity int
}

func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		rooms:         make(map[uint32]*Room),
		sessionToRoom: make(map[gocql.UUID]*uint32),
		ctx:           ctx,
		cancel:        cancel,

		maxRoomCapacity: 1024,
	}
}

func (m *Manager) generateRoomID() (uint32, error) {
	for attempts := 0; attempts < 100; attempts++ {
		var b [4]byte
		if _, err := rand.Read(b[:]); err != nil {
			return 0, err
		}
		roomID := binary.LittleEndian.Uint32(b[:])
		if _, exists := m.rooms[roomID]; !exists {
			return roomID, nil
		}
	}
	return 0, errors.New("failed to generate unique room ID")
}

func (m *Manager) CreateRoom(ownerID gocql.UUID, name string) (*Room, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, room := range m.rooms {
		if room.OwnerID == ownerID {
			return nil, errors.New("user already owns a room")
		}
	}

	if len(m.rooms) >= m.maxRoomCapacity {
		return nil, errors.New("maximum room capacity reached")
	}

	roomID, err := m.generateRoomID()
	if err != nil {
		return nil, err
	}

	room := NewRoom(roomID, ownerID, name)
	m.rooms[roomID] = room
	room.Start()

	return room, nil
}

func (m *Manager) GetRoom(roomID uint32) (*Room, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	room, exists := m.rooms[roomID]

	if !exists {
		return nil, ErrRoomNotFound
	}

	return room, nil
}

func (m *Manager) GetRoomByOwner(ownerID gocql.UUID) (*Room, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, room := range m.rooms {
		if room.OwnerID == ownerID {
			return room, nil
		}
	}

	return nil, ErrRoomNotFound
}

func (m *Manager) GetAllRooms() []*Room {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rooms := make([]*Room, 0, len(m.rooms))
	for _, room := range m.rooms {
		rooms = append(rooms, room)
	}

	return rooms
}

func (m *Manager) DeleteRoom(roomID uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	room, exists := m.rooms[roomID]
	if !exists {
		return ErrRoomNotFound
	}

	room.Shutdown()
	delete(m.rooms, roomID)

	return nil
}

func (m *Manager) GetRoomBySession(sessionID gocql.UUID) (*Room, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	roomID, exists := m.sessionToRoom[sessionID]

	if !exists {
		return nil, ErrRoomNotFound
	}

	room, exists := m.rooms[*roomID]

	if !exists {
		return nil, ErrRoomNotFound
	}

	return room, nil
}

func (m *Manager) AddSessionToRoom(roomID uint32, session *network.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	room, exists := m.rooms[roomID]
	if !exists {
		return ErrRoomNotFound
	}

	if err := room.AddSession(session); err != nil {
		return err
	}

	m.sessionToRoom[session.ID] = &roomID
	return nil
}

func (m *Manager) RemoveSessionFromRoom(sessionID gocql.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	roomID, exists := m.sessionToRoom[sessionID]
	if !exists {
		return nil // already removed
	}

	room, exists := m.rooms[*roomID]
	if exists {
		if err := room.RemoveSession(sessionID); err != nil {
			return err
		}
	}

	delete(m.sessionToRoom, sessionID)
	return nil
}
