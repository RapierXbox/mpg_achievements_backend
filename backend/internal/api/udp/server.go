package udp

import (
	"backend/internal/game"
	"backend/internal/service"
	"backend/pkg/config"
	"backend/pkg/network"
	"backend/pkg/protocol"
	"backend/pkg/utils"
	"context"
	"encoding/binary"
	"log"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/gocql/gocql"
	"github.com/prometheus/client_golang/prometheus"
)

type UDPServer struct {
	conn          *net.UDPConn
	gameManager   *game.Manager
	addr          string
	addrToSession map[string]gocql.UUID
	sessionToAddr map[gocql.UUID]*net.UDPAddr

	packetChan chan *network.UDPPacket

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
		packetChan:     make(chan *network.UDPPacket, 500), // 500 should be enough to handle some short bursts
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

	s.conn = conn

	s.logger.Printf("UDP server listening on %s with %d workers", s.addr, runtime.NumCPU()*2)

	// start worker pool
	for i := 0; i < runtime.NumCPU()*2; i++ {
		go s.worker()
	}

	go s.receiveLoop()
	return nil
}

func (s *UDPServer) RemoveSession(sessionID gocql.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	addr, exists := s.sessionToAddr[sessionID]
	if exists {
		delete(s.addrToSession, addr.String())
		delete(s.sessionToAddr, sessionID)
		udpActiveSessions.Dec()
	}
}

func (s *UDPServer) worker() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case packet := <-s.packetChan:
			s.handlePacket(packet)
		}
	}
}
func (s *UDPServer) receiveLoop() {
	buffer := make([]byte, 2048) // should be enough for our biggest packet

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Println("UDP server shutting down receive loop")
			return
		default:
			n, addr, err := s.conn.ReadFromUDP(buffer)
			if err != nil {
				s.logger.Printf("UDP server failed to read data: %v", err)
				udpPacketErrorsTotal.WithLabelValues("read", err.Error()).Inc()
				continue
			}

			data := make([]byte, n)
			copy(data, buffer[:n])

			// non-blocking send to worker pool
			select {
			case s.packetChan <- &network.UDPPacket{Address: addr, Payload: data}:
			default:
				// buffer full, drop packet
				udpPacketErrorsTotal.WithLabelValues("read", "buffer_full").Inc()
			}
		}
	}
}

func (s *UDPServer) handlePacket(packet *network.UDPPacket) {
	if len(packet.Payload) < 7 {
		s.logger.Printf("UDP server received invalid packet from %s", packet.Address.String())
		udpPacketErrorsTotal.WithLabelValues("parse", "packet_too_small").Inc()
		return
	}

	magic := string(packet.Payload[0:3])
	if magic != protocol.MagicBytes {
		s.logger.Printf("UDP server received packet without magic bytes from %s, got %s", packet.Address.String(), magic)
		udpPacketErrorsTotal.WithLabelValues("parse", "invalid_magic_bytes").Inc()
		return
	}

	msgType := packet.Payload[3]
	length := binary.LittleEndian.Uint32(packet.Payload[4:8])

	if int(length) > len(packet.Payload)-8 {
		udpPacketErrorsTotal.WithLabelValues("parse", "invalid_length").Inc()
		return
	}
	payload := packet.Payload[8 : 8+length]

	// record received packet metrics
	typeLabel := getUDPMessageTypeLabel(msgType)
	udpPacketsReceivedTotal.WithLabelValues(typeLabel).Inc()
	packetSize := float64(len(packet.Payload))
	udpPacketReceivedSizeBytes.WithLabelValues(typeLabel).Observe(packetSize)
	udpPacketReceivedSizeSummary.WithLabelValues(typeLabel).Observe(packetSize)
	udpBytesReceivedTotal.WithLabelValues(typeLabel).Add(packetSize)

	// track processing time
	timer := prometheus.NewTimer(udpPacketProcessingDuration.WithLabelValues(typeLabel))
	defer timer.ObserveDuration()

	switch msgType {
	case protocol.MsgTypeHello:
		s.handleHello(packet.Address, payload)

	case protocol.MsgTypeEntityMove:
		s.handleEntityMove(packet.Address, payload)

	case protocol.MsgTypePing:
		s.handlePing(packet.Address, payload)
	}
}

func (s *UDPServer) handleHello(addr *net.UDPAddr, payload []byte) {
	msg := &protocol.Hello{}
	if err := msg.Decode(payload); err != nil {
		s.logger.Printf("Failed to decode hello: %v", err)
		udpPacketErrorsTotal.WithLabelValues("hello", "decode_error").Inc()
		return
	}

	// parse and validate token
	claims, err := utils.ParseToken(string(msg.AccessToken), []byte(s.cfg.JWTSecret))
	if err != nil {
		s.logger.Printf("invalid token - %v", err)
		udpPacketErrorsTotal.WithLabelValues("hello", "invalid_token").Inc()
		return
	}

	// extract user ID from claims
	userID, err := gocql.ParseUUID(claims["sub"].(string))
	if err != nil {
		s.logger.Printf("invalid user claim - %v", err)
		udpPacketErrorsTotal.WithLabelValues("hello", "invalid_user_claim").Inc()
		return
	}

	valid, err := s.sessionService.CheckSession(userID, msg.DeviceID)
	if err != nil || !valid {
		s.logger.Printf("invalid session - %v", err)
		udpPacketErrorsTotal.WithLabelValues("hello", "invalid_session").Inc()
		return
	}

	room, err := s.gameManager.GetRoom(msg.RoomID)
	if err != nil {
		s.logger.Printf("Room not found: %v", err)
		udpPacketErrorsTotal.WithLabelValues("hello", "room_not_found").Inc()
		return
	}

	session, exists := room.GetSession(userID)
	if !exists {
		room.GetLogger().Printf("Session not found for user %s", userID.String())
		udpPacketErrorsTotal.WithLabelValues("hello", "session_not_found").Inc()
		return
	}

	addrKey := addr.String()

	s.mu.Lock()
	// check if this is a new session
	_, wasExisting := s.addrToSession[addrKey]
	s.addrToSession[addrKey] = userID
	s.sessionToAddr[userID] = addr
	if !wasExisting {
		udpActiveSessions.Inc()
	}
	s.mu.Unlock()

	room.GetLogger().Printf("Registered UDP address %s for session %s", addr.String(), session.ID.String())
}

func (s *UDPServer) handleEntityMove(addr *net.UDPAddr, payload []byte) {
	s.mu.RLock()
	sessionID, exists := s.addrToSession[addr.String()]
	s.mu.RUnlock()

	if !exists {
		udpPacketErrorsTotal.WithLabelValues("entity_move", "session_not_found").Inc()
		return
	}

	room, err := s.gameManager.GetRoomBySession(sessionID)
	if err != nil {
		s.logger.Printf("Room not found for session %s: %v", sessionID.String(), err)
		udpPacketErrorsTotal.WithLabelValues("entity_move", "room_not_found").Inc()
		return
	}

	session, exists := room.GetSession(sessionID)
	if !exists {
		room.GetLogger().Printf("Session not found for user %s", sessionID.String())
		udpPacketErrorsTotal.WithLabelValues("entity_move", "session_not_found").Inc()
		return
	}

	session.UpdateLastSeen()

	msg := &protocol.EntityMove{}
	if err := msg.Decode(payload); err != nil {
		room.GetLogger().Printf("Failed to decode EntityMove from %s: %v", addr.String(), err)
		udpPacketErrorsTotal.WithLabelValues("entity_move", "decode_error").Inc()
		return
	}

	entity, exists := room.GetEntity(msg.EntityID)
	if !exists {
		room.GetLogger().Printf("Entity not found for session %s", session.ID.String())
		udpPacketErrorsTotal.WithLabelValues("entity_move", "entity_not_found").Inc()
		return
	}

	timeSinceLastUpdate := time.Since(entity.LastUpdated)
	if timeSinceLastUpdate < room.MinPacketInterval { // rate limiting
		udpPacketsRateLimited.WithLabelValues("entity_move").Inc()
		return
	}

	err = room.UpdateEntityPosition(msg.EntityID, msg.Position, msg.Rotation)
	if err != nil {
		room.GetLogger().Printf("Failed to update entity: %v", err)
		udpPacketErrorsTotal.WithLabelValues("entity_move", "update_failed").Inc()
		return
	}

	s.broadcastToRoom(room, msg, sessionID)
}

func (s *UDPServer) handlePing(addr *net.UDPAddr, _ []byte) {
	pong := &protocol.Pong{}

	s.sendToAddr(addr, pong)
}

func (s *UDPServer) sendToAddr(addr *net.UDPAddr, msg protocol.Message) {
	data, err := msg.Encode()
	if err != nil {
		s.logger.Printf("Failed to encode message: %v", err)

		udpPacketSendErrorsTotal.WithLabelValues(getUDPMessageTypeLabel(msg.Type()), "encode_error").Inc()
		return
	}

	_, err = s.conn.WriteToUDP(data, addr)
	if err != nil {
		udpPacketSendErrorsTotal.WithLabelValues(getUDPMessageTypeLabel(msg.Type()), "write_error").Inc()
		return
	}

	trackSentUDPPacket(msg.Type(), len(data))
}

func (s *UDPServer) broadcastToRoom(room *game.Room, msg protocol.Message, excludeID gocql.UUID) {
	// pre-encode message once for broadcast
	data, err := msg.Encode()
	if err != nil {
		s.logger.Printf("Failed to encode message for broadcast: %v", err)
		udpPacketSendErrorsTotal.WithLabelValues(getUDPMessageTypeLabel(msg.Type()), "encode_error").Inc()
		return
	}

	sentCount := 0

	room.GetMutex().RLock()
	sessions := make([]*network.Session, 0, len(room.Sessions))
	for _, session := range room.Sessions {
		sessions = append(sessions, session)
	}
	room.GetMutex().RUnlock()

	for _, session := range sessions {
		if session.ID == excludeID {
			continue
		}

		s.mu.RLock()
		udpAddr, exists := s.sessionToAddr[session.ID]
		s.mu.RUnlock()

		if exists && udpAddr != nil {
			_, err := s.conn.WriteToUDP(data, udpAddr)
			if err != nil {
				udpPacketSendErrorsTotal.WithLabelValues(getUDPMessageTypeLabel(msg.Type()), "write_error").Inc()
			} else {
				sentCount++
			}
		}
	}

	// track metrics for all sent packets
	for i := 0; i < sentCount; i++ {
		trackSentUDPPacket(msg.Type(), len(data))
	}
}

func (s *UDPServer) Shutdown(ctx context.Context) error {
	s.cancel()

	// update active sessions gauge
	s.mu.RLock()
	sessionCount := len(s.addrToSession)
	s.mu.RUnlock()
	udpActiveSessions.Sub(float64(sessionCount))

	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}
