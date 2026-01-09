package udp

import (
	"backend/internal/game"
	"backend/internal/service"
	"backend/pkg/config"
	"backend/pkg/protocol"
	"backend/pkg/utils"
	"context"
	"encoding/binary"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/gocql/gocql"
)

type UDPServer struct {
	conn          *net.UDPConn
	gameManager   *game.Manager
	addr          string
	addrToSession map[string]gocql.UUID
	sessionToAddr map[gocql.UUID]*net.UDPAddr

	logger         *log.Logger
	cfg            *config.Config
	sessionService *service.SessionService

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

func NewUDPServer(addr string, gameManager *game.Manager, cfg *config.Config, sessionService *service.SessionService) *UDPServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &UDPServer{
		addr:           addr,
		gameManager:    gameManager,
		cfg:            cfg,
		sessionService: sessionService,
		logger:         log.New(os.Stdout, "UDP SERVER: ", log.Ldate|log.Ltime|log.Lshortfile),
		addrToSession:  make(map[string]gocql.UUID),
		sessionToAddr:  make(map[gocql.UUID]*net.UDPAddr),
		ctx:            ctx,
		cancel:         cancel,
	}
}

func (s *UDPServer) Start() error {
	udpAddr, err := net.ResolveUDPAddr("udp", s.addr)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}

	conn.SetReadBuffer(4 * 1024 * 1024)
	conn.SetWriteBuffer(4 * 1024 * 1024)
	s.conn = conn

	s.logger.Printf("UDP server listening on %s", s.addr)

	go s.recieveLoop()
	return nil
}

func (s *UDPServer) recieveLoop() {
	buffer := make([]byte, 2048)

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Println("UDP server shutting down receive loop")
			return
		default:
			n, addr, err := s.conn.ReadFromUDP(buffer)
			if err != nil {
				s.logger.Printf("UDP server failed to read data: %v", err)
				continue
			}

			data := make([]byte, n)
			copy(data, buffer[:n])

			go s.handlePacket(addr, data)
		}
	}
}

func (s *UDPServer) handlePacket(addr *net.UDPAddr, data []byte) {
	if len(data) < 7 {
		s.logger.Printf("UDP server received invalid packet from %s", addr.String())
		return
	}

	magic := string(data[0:3])
	if magic != protocol.MagicBytes {
		s.logger.Printf("UDP server received packet without magic bytes from %s, got %s", addr.String(), magic)
		return
	}

	msgType := data[3]
	length := binary.LittleEndian.Uint32(data[4:8])
	payload := data[8 : 8+length]

	switch msgType {
	case protocol.MsgTypeHello:
		s.handleHello(addr, payload)

	case protocol.MsgTypeEntityMove:
		s.handleEntityMove(addr, payload)

	case protocol.MsgTypePing:
		s.handlePing(addr, payload)
	}
}

func (s *UDPServer) handleHello(addr *net.UDPAddr, payload []byte) {
	msg := &protocol.Hello{}
	if err := msg.Decode(payload); err != nil {
		s.logger.Printf("Failed to decode hello: %v", err)
		return
	}

	// parse and validate token
	claims, err := utils.ParseToken(string(msg.AccessToken), []byte(s.cfg.JWTSecret))
	if err != nil {
		s.logger.Printf("invalid token - %v", err)
		return
	}

	// extract user ID from claims
	userID, err := gocql.ParseUUID(claims["sub"].(string))
	if err != nil {
		s.logger.Printf("invalid user claim - %v", err)
		return
	}

	valid, err := s.sessionService.CheckSession(userID, msg.DeviceID)
	if err != nil || !valid {
		s.logger.Printf("invalid session - %v", err)
		return
	}

	room, err := s.gameManager.GetRoom(msg.RoomID)
	if err != nil {
		s.logger.Printf("Room not found: %v", err)
		return
	}

	session, exists := room.GetSession(userID)
	if !exists {
		room.GetLogger().Printf("Session not found for user %s", userID.String())
		return
	}

	addrKey := addr.String()

	s.mu.Lock()
	s.addrToSession[addrKey] = userID
	s.sessionToAddr[userID] = addr
	s.mu.Unlock()

	room.GetLogger().Printf("Registered UDP address %s for session %s", addr.String(), session.ID.String())
}

func (s *UDPServer) handleEntityMove(addr *net.UDPAddr, payload []byte) {
	s.mu.RLock()
	sessionID, exists := s.addrToSession[addr.String()]
	s.mu.RUnlock()

	if !exists {
		return
	}

	room, err := s.gameManager.GetRoomBySession(sessionID)
	if err != nil {
		s.logger.Printf("Room not found for session %s: %v", sessionID.String(), err)
		return
	}

	session, exists := room.GetSession(sessionID)
	if !exists {
		room.GetLogger().Printf("Session not found for user %s", sessionID.String())
		return
	}

	session.UpdateLastSeen()

	msg := &protocol.EntityMove{}
	if err := msg.Decode(payload); err != nil {
		room.GetLogger().Printf("Failed to decode EntityMove from %s: %v", addr.String(), err)
		return
	}

	entity, exists := room.GetEntity(msg.EntityID)
	if !exists {
		room.GetLogger().Printf("Entity not found for session %s", session.ID.String())
		return
	}

	entity.Position = msg.Position
	entity.Rotation = msg.Rotation

	timeSinceLastUpdate := time.Since(entity.LastUpdated)
	if timeSinceLastUpdate < room.MinPacketInterval { // rate limiting
		return
	}

	entity.LastUpdated = time.Now()
	s.broadcastToRoom(room, msg, sessionID)
}

func (s *UDPServer) handlePing(addr *net.UDPAddr, payload []byte) {
	pong := &protocol.Pong{}

	s.sendToAddr(addr, pong)
}

func (s *UDPServer) sendToAddr(addr *net.UDPAddr, msg protocol.Message) {
	data, err := msg.Encode()
	if err != nil {
		s.logger.Printf("Failed to encode message: %v", err)
		return
	}

	s.conn.WriteToUDP(data, addr)
}

func (s *UDPServer) broadcastToRoom(room *game.Room, msg protocol.Message, excludeID gocql.UUID) {
	room.GetMutex().RLock()
	defer room.GetMutex().RUnlock()

	for _, session := range room.Sessions {
		if session.ID == excludeID {
			continue
		}

		s.mu.RLock()
		udpAddr, exists := s.sessionToAddr[session.ID]
		s.mu.RUnlock()

		if exists && udpAddr != nil {
			s.sendToAddr(udpAddr, msg)
		}
	}
}

func (s *UDPServer) Shutdown(ctx context.Context) error {
	s.cancel()
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}
