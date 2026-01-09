package tcp

import (
	"backend/internal/game"
	"backend/internal/service"
	"backend/pkg/config"
	"backend/pkg/network"
	"backend/pkg/protocol"
	"backend/pkg/utils"
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"os"

	"github.com/gocql/gocql"
)

type TCPServer struct {
	listener    net.Listener
	gameManager *game.Manager
	addr        string

	logger         *log.Logger
	cfg            *config.Config
	sessionService *service.SessionService

	ctx    context.Context
	cancel context.CancelFunc
}

func NewTCPServer(addr string, gameManager *game.Manager, cfg *config.Config, sessionService *service.SessionService) *TCPServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &TCPServer{
		addr:           addr,
		gameManager:    gameManager,
		cfg:            cfg,
		sessionService: sessionService,
		logger:         log.New(os.Stdout, "TCP SERVER: ", log.Ldate|log.Ltime|log.Lshortfile),

		ctx:    ctx,
		cancel: cancel,
	}
}

func (s *TCPServer) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.listener = ln

	s.logger.Printf("TCP server listening on %s", s.addr)

	go s.acceptLoop()
	return nil
}

func (s *TCPServer) acceptLoop() {
	for {
		select {
		case <-s.ctx.Done():
			s.logger.Println("TCP server shutting down accept loop")
			return
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				s.logger.Printf("TCP server failed to accept connection: %v", err)
				continue
			}

			go s.handleConnection(conn)
		}
	}
}

func (s *TCPServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	s.logger.Printf("New TCP connection from %s", conn.RemoteAddr().String())

	reader := bufio.NewReader(conn)

	var sessionID gocql.UUID
	var room *game.Room

	for {
		msgType, payload, err := s.readMessage(reader)
		if err != nil {
			if err != io.EOF {
				s.logger.Printf("Error reading message: %v", err)
			}
			break
		}

		switch msgType {
		case protocol.MsgTypeHello:
			sessionID, room = s.handleHello(conn, payload)
			if sessionID == (gocql.UUID{}) {
				return
			}

		case protocol.MsgTypeNewEntity:
			if room != nil && sessionID != (gocql.UUID{}) {
				s.handleNewEntity(room, sessionID, payload)
			}

		case protocol.MsgTypeCustomData:
			if room != nil && sessionID != (gocql.UUID{}) {
				s.handleCustomData(room, sessionID, payload)
			}

		case protocol.MsgTypeRemoveEntity:
			if room != nil && sessionID != (gocql.UUID{}) {
				s.handleRemoveEntity(room, sessionID, payload)
			}

		case protocol.MsgTypeRequestChunkData:
			if room != nil && sessionID != (gocql.UUID{}) {
				s.handleRequestChunkData(room, sessionID, payload)
			}
		case protocol.MsgTypeChunkData:
			if room != nil && sessionID != (gocql.UUID{}) {
				s.handleChunkData(room, sessionID, payload)
			}
		case protocol.MsgTypeChatMessage:
			if room != nil && sessionID != (gocql.UUID{}) {
				s.handleChatMessage(room, sessionID, payload)
			}
		}
	}

	// cleanup session on conn close
	if sessionID != (gocql.UUID{}) {
		s.gameManager.RemoveSessionFromRoom(sessionID)
		if room != nil {
			// broadcast entity removals for all entities owned by this session
			room.GetMutex().RLock()
			entitiesToRemove := []gocql.UUID{}
			for _, entity := range room.Entities {
				if entity.ParentID == sessionID {
					entitiesToRemove = append(entitiesToRemove, entity.ID)
				}
			}
			room.GetMutex().RUnlock()

			for _, entityID := range entitiesToRemove {
				room.RemoveEntity(entityID)
				removeMsg := &protocol.RemoveEntity{EntityID: entityID}
				room.BroadcastTCP(removeMsg)
			}
		}
	}
}

func (s *TCPServer) readMessage(reader *bufio.Reader) (uint8, []byte, error) {
	// read magic bytes
	magic := make([]byte, 3)
	_, err := io.ReadFull(reader, magic)
	if err != nil {
		return 0, nil, err
	}

	if string(magic) != protocol.MagicBytes {
		return 0, nil, errors.New("invalid magic bytes")
	}

	// read message type
	msgTypeBuf := make([]byte, 1)
	_, err = io.ReadFull(reader, msgTypeBuf)
	if err != nil {
		return 0, nil, err
	}
	msgType := msgTypeBuf[0]

	// read payload length
	lengthBuf := make([]byte, 4)
	_, err = io.ReadFull(reader, lengthBuf)
	if err != nil {
		return 0, nil, err
	}
	length := binary.LittleEndian.Uint32(lengthBuf)

	// read payload
	payload := make([]byte, length)
	_, err = io.ReadFull(reader, payload)
	if err != nil {
		return 0, nil, err
	}

	return msgType, payload, nil
}

func (s *TCPServer) handleHello(conn net.Conn, payload []byte) (gocql.UUID, *game.Room) {
	msg := &protocol.Hello{}
	if err := msg.Decode(payload); err != nil {
		s.logger.Printf("Failed to decode hello: %v", err)
		conn.Close()
		return gocql.UUID{}, nil
	}

	// parse and validate token
	claims, err := utils.ParseToken(string(msg.AccessToken), []byte(s.cfg.JWTSecret))
	if err != nil {
		s.logger.Printf("invalid token - %v", err)
		conn.Close()
		return gocql.UUID{}, nil
	}

	// extract user ID from claims
	userID, err := gocql.ParseUUID(claims["sub"].(string))
	if err != nil {
		s.logger.Printf("invalid user claim - %v", err)
		conn.Close()
		return gocql.UUID{}, nil
	}

	valid, err := s.sessionService.CheckSession(userID, msg.DeviceID)
	if err != nil || !valid {
		s.logger.Printf("invalid session - %v", err)
		conn.Close()
		return gocql.UUID{}, nil
	}

	room, err := s.gameManager.GetRoom(msg.RoomID)
	if err != nil {
		s.logger.Printf("Room not found: %v", err)
		conn.Close()
		return gocql.UUID{}, nil
	}

	session := network.NewSession(userID)
	session.TCPConn = conn

	if err := s.gameManager.AddSessionToRoom(msg.RoomID, session); err != nil {
		room.GetLogger().Printf("Failed to add session: %v", err)
		conn.Close()
		return gocql.UUID{}, nil
	}

	room.GetLogger().Printf("Session %s connected", session.ID.String())

	helloAckMsg := protocol.HelloAck{
		Success: true,
	}

	room.SendTCPToSession(session, &helloAckMsg)

	// send all newentity messages for already existing entities
	room.GetMutex().RLock()
	entitiesToSend := make([]protocol.NewEntity, 0, len(room.Entities))
	for _, entity := range room.Entities {
		entitiesToSend = append(entitiesToSend, protocol.NewEntity{
			EntityID:   entity.ID,
			EntityType: entity.Type,
			Position:   entity.Position,
			Rotation:   entity.Rotation,
			CustomData: entity.CustomData,
		})
	}
	room.GetMutex().RUnlock()

	for _, newEntityMsg := range entitiesToSend {
		room.SendTCPToSession(session, &newEntityMsg)
	}

	return userID, room
}

func (s *TCPServer) handleNewEntity(room *game.Room, sessionID gocql.UUID, payload []byte) {
	msg := &protocol.NewEntity{}
	if err := msg.Decode(payload); err != nil {
		room.GetLogger().Printf("Failed to decode NewEntity: %v", err)
		return
	}

	room.GetMutex().RLock()
	var numEntitysForSession int
	for _, entity := range room.Entities {
		if entity.ParentID == sessionID {
			numEntitysForSession++
		}
	}
	room.GetMutex().RUnlock()

	if numEntitysForSession >= room.MaxEntitysPerSession {
		room.GetLogger().Printf("Session %s has reached maximum entity limit", sessionID.String())
		return
	}

	summonedEntity := game.NewEntity(
		msg.EntityID,
		sessionID,
		msg.EntityType,
		msg.Position,
		msg.Rotation,
		msg.CustomData,
	)

	err := room.AddEntity(&summonedEntity)
	if err != nil {
		room.GetLogger().Printf("Error adding Entity: %s", err.Error())
		return
	}

	newEntityMsg := &protocol.NewEntity{
		EntityID:   summonedEntity.ID,
		EntityType: summonedEntity.Type,
		Position:   summonedEntity.Position,
		Rotation:   summonedEntity.Rotation,
		CustomData: summonedEntity.CustomData,
	}

	room.BroadcastTCP(newEntityMsg)
}

func (s *TCPServer) handleCustomData(room *game.Room, sessionID gocql.UUID, payload []byte) {
	msg := &protocol.CustomData{}
	if err := msg.Decode(payload); err != nil {
		room.GetLogger().Printf("Failed to decode CustomData: %v", err)
		return
	}

	targetEntity, exists := room.GetEntity(msg.EntityID)

	if msg.EntityID == sessionID { // if the message is about the session itself allow it
		updateMsg := &protocol.CustomData{
			EntityID:   msg.EntityID,
			CustomData: msg.CustomData,
		}
		room.BroadcastTCP(updateMsg)
		return
	}

	// otherwise the sessions needs to be the owner of the entity
	if !exists {
		room.GetLogger().Printf("Entity %s not found for CustomData", msg.EntityID.String())
		return
	}

	if targetEntity.ParentID != sessionID {
		room.GetLogger().Printf("Session %s unauthorized to update Entity %s", sessionID.String(), msg.EntityID.String())
		return
	}

	updateMsg := &protocol.CustomData{
		EntityID:   msg.EntityID,
		CustomData: msg.CustomData,
	}

	room.BroadcastTCP(updateMsg)
}

func (s *TCPServer) handleRemoveEntity(room *game.Room, sessionID gocql.UUID, payload []byte) {
	msg := &protocol.RemoveEntity{}
	if err := msg.Decode(payload); err != nil {
		room.GetLogger().Printf("Failed to decode RemoveEntity: %v", err)
		return
	}

	targetEntity, exists := room.GetEntity(msg.EntityID)
	if !exists {
		room.GetLogger().Printf("Entity %s not found for removal", msg.EntityID.String())
		return
	}

	if targetEntity.ParentID != sessionID {
		room.GetLogger().Printf("Session %s unauthorized to remove Entity %s", sessionID.String(), msg.EntityID.String())
		return
	}

	if err := room.RemoveEntity(msg.EntityID); err != nil {
		room.GetLogger().Printf("Failed to remove entity: %v", err)
		return
	}

	removeMsg := &protocol.RemoveEntity{
		EntityID: msg.EntityID,
	}
	room.BroadcastTCP(removeMsg)
}

func (s *TCPServer) handleChatMessage(room *game.Room, sessionID gocql.UUID, payload []byte) {
	msg := &protocol.ChatMessage{}
	if err := msg.Decode(payload); err != nil {
		room.GetLogger().Printf("Failed to decode ChatMessage: %v", err)
		return
	}

	chatMsg := &protocol.ChatMessage{
		SenderID: sessionID,
		Message:  msg.Message,
	}

	room.BroadcastTCP(chatMsg)
}

func (s *TCPServer) handleRequestChunkData(room *game.Room, sessionID gocql.UUID, payload []byte) {
	msg := &protocol.RequestChunkData{}
	if err := msg.Decode(payload); err != nil {
		room.GetLogger().Printf("Failed to decode RequestChunkData: %v", err)
		return
	}

	chunkPosition := utils.Vector2I32{
		X: msg.ChunkX,
		Y: msg.ChunkY,
	}

	chunk, exists := room.Chunks[chunkPosition]
	if !exists {
		room.GetLogger().Printf("Chunk at position %v not found... requesting it from room owner!", chunkPosition)

		// Request chunk data from room owner
		requestMsg := &protocol.RequestChunkData{
			ChunkX: chunkPosition.X,
			ChunkY: chunkPosition.Y,
		}

		ownerSession, exists := room.GetSession(room.OwnerID)
		if exists {
			room.SendTCPToSession(ownerSession, requestMsg)
		}
		return
	}

	chunkDataMsg := &protocol.ChunkData{
		ChunkX:    chunkPosition.X,
		ChunkY:    chunkPosition.Y,
		ChunkData: chunk.Data,
	}

	senderSession, exists := room.GetSession(sessionID)
	if !exists {
		return
	}

	room.SendTCPToSession(senderSession, chunkDataMsg)
}

func (s *TCPServer) handleChunkData(room *game.Room, sessionID gocql.UUID, payload []byte) {
	msg := &protocol.ChunkData{}
	if err := msg.Decode(payload); err != nil {
		room.GetLogger().Printf("Failed to decode ChunkData: %v", err)
		return
	}

	if sessionID != room.OwnerID {
		room.GetLogger().Printf("Session %s unauthorized to send ChunkData", sessionID.String())
		return
	}

	chunkPosition := utils.Vector2I32{
		X: msg.ChunkX,
		Y: msg.ChunkY,
	}

	room.Chunks[chunkPosition] = &game.Chunk{
		Data: msg.ChunkData,
	}
}

func (s *TCPServer) Shutdown(ctx context.Context) error {
	s.cancel()
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}
