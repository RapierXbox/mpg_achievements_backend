package game

import (
	"backend/pkg/network"
	"backend/pkg/protocol"
	"backend/pkg/utils"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gocql/gocql"
)

type Room struct {
	ID      uint32
	OwnerID gocql.UUID
	Name    string

	Sessions map[gocql.UUID]*network.Session
	Entities map[gocql.UUID]*Entity
	Chunks   map[utils.Vector2I32]*Chunk

	TCPBroadcast chan *protocol.Message
	UDPBroadcast chan *protocol.Message

	TickRate             int
	MinPacketInterval    time.Duration
	MaxEntitysPerSession int

	logger *log.Logger

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

func NewRoom(id uint32, ownerID gocql.UUID, name string) *Room {
	ctx, cancel := context.WithCancel(context.Background())
	return &Room{
		ID:                   id,
		OwnerID:              ownerID,
		Name:                 name,
		Sessions:             make(map[gocql.UUID]*network.Session),
		Entities:             make(map[gocql.UUID]*Entity),
		Chunks:               make(map[utils.Vector2I32]*Chunk),
		TCPBroadcast:         make(chan *protocol.Message, 100),
		UDPBroadcast:         make(chan *protocol.Message, 500),
		TickRate:             10,
		MinPacketInterval:    50 * time.Millisecond,
		MaxEntitysPerSession: 50,
		logger:               log.New(os.Stdout, fmt.Sprintf("ROOM (%d): ", id), log.Ldate|log.Ltime|log.Lshortfile),
		ctx:                  ctx,
		cancel:               cancel,
	}
}

func (r *Room) Start() {
	go r.gameLoop()
	go r.tcpBroadcaster()
}

func (r *Room) gameLoop() {
	ticker := time.NewTicker(time.Second / time.Duration(r.TickRate))
	defer ticker.Stop()

	tickCount := 0

	for {
		select {
		case <-ticker.C:
			tickCount++

			// todo

		case <-r.ctx.Done():
			return
		}
	}
}

// adds entity to rooms entites
func (r *Room) AddEntity(entity *Entity) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, exists := r.Entities[entity.ID]
	if exists {
		return fmt.Errorf("Entity with id (%s) already exists", entity.ID.String())
	}
	r.Entities[entity.ID] = entity
	return nil
}

// remove entity by entity id
func (r *Room) RemoveEntity(entityID gocql.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, exists := r.Entities[entityID]
	if !exists {
		return errors.New("entity not found")
	}

	delete(r.Entities, entityID)

	return nil
}

func (r *Room) GetEntity(entityID gocql.UUID) (*Entity, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entity, exists := r.Entities[entityID]
	return entity, exists
}

func (r *Room) AddSession(session *network.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.Sessions) >= 1024 {
		return ErrRoomFull
	}

	session.LastSeen = time.Now()
	r.Sessions[session.ID] = session

	return nil
}

func (r *Room) RemoveSession(sessionID gocql.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.Sessions[sessionID]
	if !exists {
		return ErrPlayerNotFound
	}

	if session.TCPConn != nil {
		session.TCPConn.Close()
	}

	delete(r.Sessions, sessionID)

	return nil
}

func (r *Room) GetSession(sessionID gocql.UUID) (*network.Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	session, exists := r.Sessions[sessionID]
	return session, exists
}

func (r *Room) BroadcastTCP(msg protocol.Message) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.broadcastTCPUnsafe(msg)
}

func (r *Room) BroadcastUDP(msg protocol.Message) {
	select {
	case r.UDPBroadcast <- &msg:
	default:
	}
}

func (r *Room) BroadcastTCPExcept(excludeSessionID gocql.UUID, msg protocol.Message) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for sessionID, session := range r.Sessions {
		if sessionID == excludeSessionID {
			continue
		}

		if session.TCPConn != nil {
			go r.SendTCPToSession(session, msg)
		}
	}
}

func (r *Room) broadcastTCPUnsafe(msg protocol.Message) {
	for _, session := range r.Sessions {
		if session.TCPConn != nil {
			go r.SendTCPToSession(session, msg)
		}
	}
}

func (r *Room) tcpBroadcaster() {
	for {
		select {
		case msgPtr := <-r.TCPBroadcast:
			if msgPtr == nil {
				continue
			}
			msg := *msgPtr

			r.mu.RLock()
			for _, session := range r.Sessions {
				if session.TCPConn != nil {
					go r.SendTCPToSession(session, msg)
				}
			}
			r.mu.RUnlock()

		case <-r.ctx.Done():
			return
		}
	}
}

func (r *Room) SendTCPToSession(session *network.Session, msg protocol.Message) {
	data, err := msg.Encode()
	if err != nil {
		return
	}

	session.TCPConn.Write(data)
}

func (r *Room) GetLogger() *log.Logger {
	return r.logger
}

// returns pointer to private mutex for thread safe operations from the tcp server... i could also just make mu uppercase
func (r *Room) GetMutex() *sync.RWMutex {
	return &r.mu
}

func (r *Room) Shutdown() {
	r.cancel()
	close(r.TCPBroadcast)
	close(r.UDPBroadcast)
}
