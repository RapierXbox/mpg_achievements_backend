package network

import (
	"net"
	"sync"
	"time"

	"github.com/gocql/gocql"
)

type Session struct {
	ID gocql.UUID

	TCPConn  net.Conn
	UDPAddr  *net.UDPAddr
	LastSeen time.Time
	mu       sync.RWMutex
}

func NewSession(id gocql.UUID) *Session {
	return &Session{
		ID:       id,
		LastSeen: time.Now(),
	}
}

func (s *Session) UpdateLastSeen() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastSeen = time.Now()
}

func (s *Session) IsActive(threshold time.Duration) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.LastSeen) < threshold
}
