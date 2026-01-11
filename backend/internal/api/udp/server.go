//go:build linux
// +build linux

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
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/gocql/gocql"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sys/unix"
)

type UDPServer struct {
	socket        int
	gameManager   *game.Manager
	addr          string
	addrToSession map[string]sessionInfo
	sessionToAddr map[gocql.UUID]*net.UDPAddr

	packetChan chan *network.UDPPacket

	logger         *log.Logger
	cfg            *config.Config
	sessionService *service.SessionService

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

type sessionInfo struct {
	sessionID gocql.UUID
	room      *game.Room // cache room pointer
}

func NewUDPServer(addr string, gameManager *game.Manager, cfg *config.Config, sessionService *service.SessionService) *UDPServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &UDPServer{
		addr:           addr,
		gameManager:    gameManager,
		cfg:            cfg,
		sessionService: sessionService,
		logger:         log.New(os.Stdout, "UDP SERVER: ", log.Ldate|log.Ltime|log.Lshortfile),
		addrToSession:  make(map[string]sessionInfo),
		sessionToAddr:  make(map[gocql.UUID]*net.UDPAddr),
		packetChan:     make(chan *network.UDPPacket, 500), // 500 should be enough to handle some short bursts
		ctx:            ctx,
		cancel:         cancel,
	}
}

func (s *UDPServer) Start() error {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, unix.IPPROTO_UDP)
	if err != nil {
		return err
	}

	localAddr := &unix.SockaddrInet4{
		Port: 9001,
		Addr: [4]byte{0, 0, 0, 0},
	}
	unix.Bind(fd, localAddr)

	s.socket = fd

	s.logger.Printf("UDP server listening on %s with %d workers", s.addr, runtime.NumCPU()*4)

	// start worker pool
	for i := 0; i < runtime.NumCPU()*4; i++ {
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

// package level preallocation
var packetPool = sync.Pool{
	New: func() interface{} {
		return &network.UDPPacket{
			Payload: make([]byte, 0, 256),
		}
	},
}

func (s *UDPServer) worker() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case packet := <-s.packetChan:
			s.handlePacket(packet)
			packetPool.Put(packet) // return to pool after processing
		}
	}
}

func (s *UDPServer) receiveFromUDP(buffer []byte) (int, *net.UDPAddr, error) {
	var iovec unix.Iovec
	iovec.Base = &buffer[0]
	iovec.SetLen(len(buffer))

	var sockaddr unix.RawSockaddrInet4

	msghdr := unix.Msghdr{
		Name:    (*byte)(unsafe.Pointer(&sockaddr)),
		Namelen: uint32(unsafe.Sizeof(sockaddr)),
		Iov:     &iovec,
		Iovlen:  1,
	}

	// call recvmsg
	n, _, errno := syscall.Syscall(
		unix.SYS_RECVMSG,
		uintptr(s.socket),
		uintptr(unsafe.Pointer(&msghdr)),
		0, // flags
	)

	if errno != 0 {
		return 0, nil, fmt.Errorf("recvmsg failed: %v", errno)
	}

	// parse the sockaddr to get the UDP address
	port := uint16(sockaddr.Port)
	port = (port>>8 | port<<8) // convert from network byte order
	udpAddr := &net.UDPAddr{
		IP:   net.IPv4(sockaddr.Addr[0], sockaddr.Addr[1], sockaddr.Addr[2], sockaddr.Addr[3]),
		Port: int(port),
	}

	return int(n), udpAddr, nil
}

func (s *UDPServer) receiveLoop() {
	buffer := make([]byte, 256) // should be enough for our biggest packet (hello)

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Println("UDP server shutting down receive loop")
			return
		default:
			n, addr, err := s.receiveFromUDP(buffer)
			if err != nil {
				s.logger.Printf("UDP server failed to read data: %v", err)
				udpPacketErrorsTotal.WithLabelValues("read", err.Error()).Inc()
				continue
			}

			// use sync.Pool for packet allocation
			packet := packetPool.Get().(*network.UDPPacket)
			packet.Address = addr
			if cap(packet.Payload) < n {
				packet.Payload = make([]byte, n)
			} else {
				packet.Payload = packet.Payload[:n]
			}
			copy(packet.Payload, buffer[:n])

			select {
			case s.packetChan <- packet:
			default:
				packetPool.Put(packet) // return to pool if dropped
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
	s.addrToSession[addrKey] = sessionInfo{sessionID: userID, room: room}
	s.sessionToAddr[userID] = addr
	if !wasExisting {
		udpActiveSessions.Inc()
	}
	s.mu.Unlock()

	room.GetLogger().Printf("Registered UDP address %s for session %s", addr.String(), session.ID.String())
}

func (s *UDPServer) handleEntityMove(addr *net.UDPAddr, payload []byte) {
	s.mu.RLock()
	info, exists := s.addrToSession[addr.String()]
	s.mu.RUnlock()

	if !exists {
		udpPacketErrorsTotal.WithLabelValues("entity_move", "session_not_found").Inc()
		return
	}

	room := info.room

	session, exists := room.GetSession(info.sessionID)
	if !exists {
		room.GetLogger().Printf("Session not found for user %s", info.sessionID.String())
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

	timeSinceLastUpdate := time.Since(time.Unix(0, entity.LastUpdatedNanos.Load()))
	if timeSinceLastUpdate < room.MinPacketInterval { // rate limiting
		udpPacketsRateLimited.WithLabelValues("entity_move").Inc()
		return
	}

	entity.UpdatePositon(msg.Position, msg.Rotation)

	s.broadcastUDP(room, msg, info.sessionID)
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

	sockaddr := &unix.SockaddrInet4{
		Port: addr.Port,
	}
	copy(sockaddr.Addr[:], addr.IP.To4())

	_, err = unix.SendmsgN(s.socket, data, nil, sockaddr, 0)
	if err != nil {
		udpPacketSendErrorsTotal.WithLabelValues(getUDPMessageTypeLabel(msg.Type()), "write_error").Inc()
		return
	}

	trackSentUDPPacket(msg.Type(), len(data))
}

func (s *UDPServer) broadcastUDP(room *game.Room, msg protocol.Message, excludeID gocql.UUID) {
	data, err := msg.Encode()
	if err != nil {
		s.logger.Printf("Failed to encode message for broadcast: %v", err)
		udpPacketSendErrorsTotal.WithLabelValues(getUDPMessageTypeLabel(msg.Type()), "encode_error").Inc()
		return
	}

	// pre-allocate slice for addresses
	addresses := make([]*net.UDPAddr, 0, 32)

	room.GetMutex().RLock()
	s.mu.RLock()

	for sessionID := range room.Sessions {
		if sessionID == excludeID {
			continue
		}
		if udpAddr, exists := s.sessionToAddr[sessionID]; exists && udpAddr != nil {
			addresses = append(addresses, udpAddr)
		}
	}

	s.mu.RUnlock()
	room.GetMutex().RUnlock()

	// prepare message headers for all destinations
	msgvec := make([]network.Mmsghdr, len(addresses))
	iovecs := make([]unix.Iovec, len(addresses))
	sockaddrs := make([]unix.RawSockaddrInet4, len(addresses))

	for i, addr := range addresses {
		// Set up the iovec (data buffer)
		iovecs[i] = unix.Iovec{
			Base: &data[0],
		}
		iovecs[i].SetLen(len(data))

		// Set up the sockaddr
		sockaddrs[i] = unix.RawSockaddrInet4{
			Family: unix.AF_INET,
		}
		port := uint16(addr.Port)
		sockaddrs[i].Port = (port>>8 | port<<8) // Convert to network byte order
		copy(sockaddrs[i].Addr[:], addr.IP.To4())

		// Set up the message header
		msgvec[i] = network.Mmsghdr{
			Msghdr: unix.Msghdr{
				Name:    (*byte)(unsafe.Pointer(&sockaddrs[i])),
				Namelen: uint32(unsafe.Sizeof(sockaddrs[i])),
				Iov:     &iovecs[i],
				Iovlen:  1,
			},
		}
	}

	// call sendmmsg directly via syscall
	n, _, errno := syscall.Syscall6(
		network.GetSendmmsgSyscall(),
		uintptr(s.socket),
		uintptr(unsafe.Pointer(&msgvec[0])),
		uintptr(len(msgvec)),
		0, // flags
		0, 0,
	)

	if errno != 0 {
		return
	}

	// batch metric tracking
	typeLabel := getUDPMessageTypeLabel(msg.Type())
	udpPacketsSentTotal.WithLabelValues(typeLabel).Add(float64(n))
	packetSize := float64(len(data))
	for i := 0; i < int(n); i++ {
		udpPacketSentSizeBytes.WithLabelValues(typeLabel).Observe(packetSize)
	}
	udpBytesSentTotal.WithLabelValues(typeLabel).Add(packetSize * float64(n))

}

func (s *UDPServer) Shutdown(ctx context.Context) error {
	s.cancel()

	// update active sessions gauge
	s.mu.RLock()
	sessionCount := len(s.addrToSession)
	s.mu.RUnlock()
	udpActiveSessions.Sub(float64(sessionCount))

	return unix.Close(s.socket)
}
