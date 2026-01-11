package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"perf/protocol"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gocql/gocql"
)

// before running truncate the auth.accounts table because somethimes the delete of old account fails (PLEASE DONT DO THIS IN PROD!!!!!!!) [truncate wipes all user accounts]
const (
	NumRooms             = 10
	NumSessionsPerRoom   = 100
	NumEntitysPerSession = 50
	PacketIntervalMs     = 51

	ServerAddress = "localhost"

	MaxConcurrentHTTPRequests = 150 // seems to be the limit. most probable cause is hashing of passwords taking up performance. also sometimes hitting some kind of scylla timeout. none of these should happen in prod.
)

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:          2000,
		MaxIdleConnsPerHost:   2000,
		MaxConnsPerHost:       0,
		IdleConnTimeout:       90 * time.Second,
		DisableKeepAlives:     false,
		DisableCompression:    false,
		ResponseHeaderTimeout: 20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

type User struct {
	Email        string
	Password     string
	DeviceID     gocql.UUID
	AccessToken  string
	RefreshToken string
}

type Stats struct {
	UsersCreated   atomic.Int64
	UsersLoggedIn  atomic.Int64
	RoomsCreated   atomic.Int64
	SessionsActive atomic.Int64
	TotalErrors    atomic.Int64
	UsersDeleted   atomic.Int64
}

var stats Stats

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	// track all users for cleanup
	var (
		usersMutex sync.RWMutex
		allUsers   []*User
	)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		log.Println("\nshutdown signal received - cleaning up")
		cancel() // cancel context to stop sessions

		// wait a moment for sessions to close
		time.Sleep(2 * time.Second)

		// delete all users
		deleteAllUsers(&usersMutex, allUsers)

		os.Exit(0)
	}()

	// goroutine for stats every 5s
	go reportStats(ctx)

	totalUsers := NumRooms * NumSessionsPerRoom
	log.Printf("=== starting stress test: %d users, %d rooms ===\n", totalUsers, NumRooms)

	users := make([]*User, totalUsers)
	var wg sync.WaitGroup
	httpSemaphore := make(chan struct{}, MaxConcurrentHTTPRequests)

	// create users
	startTime := time.Now()
	log.Println("phase 1: creating users...")
	for i := 0; i < totalUsers; i++ {
		wg.Add(1)
		httpSemaphore <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-httpSemaphore }()

			if user := createUser(i); user != nil {
				users[i] = user

				// add to list for cleanup
				usersMutex.Lock()
				allUsers = append(allUsers, user)
				usersMutex.Unlock()

				stats.UsersCreated.Add(1)
			} else {
				stats.TotalErrors.Add(1)
			}
		}(i)
	}
	wg.Wait()
	log.Printf("phase 1 complete: %d users created in %v\n",
		stats.UsersCreated.Load(), time.Since(startTime))

	// login users
	startTime = time.Now()
	log.Println("phase 2: logging in users...")
	for i := 0; i < totalUsers; i++ {
		if users[i] == nil {
			continue
		}

		wg.Add(1)
		httpSemaphore <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-httpSemaphore }()

			if users[i].login() {
				stats.UsersLoggedIn.Add(1)
			} else {
				stats.TotalErrors.Add(1)
			}
		}(i)
	}
	wg.Wait()
	log.Printf("phase 2 complete: %d users logged in in %v\n",
		stats.UsersLoggedIn.Load(), time.Since(startTime))

	// create rooms
	startTime = time.Now()
	log.Println("phase 3: creating rooms...")
	rooms := make([]uint32, NumRooms)
	for i := 0; i < NumRooms; i++ {
		userIndex := i * NumSessionsPerRoom
		if userIndex >= len(users) || users[userIndex] == nil {
			log.Printf("skipping room %d (user creation failed)", i)
			stats.TotalErrors.Add(1)
			continue
		}

		wg.Add(1)
		httpSemaphore <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-httpSemaphore }()

			userIndex := i * NumSessionsPerRoom
			if roomID := users[userIndex].createRoom(); roomID > 0 {
				rooms[i] = roomID
				stats.RoomsCreated.Add(1)
			} else {
				stats.TotalErrors.Add(1)
			}
		}(i)
	}
	wg.Wait()
	log.Printf("phase 3 complete: %d rooms created in %v\n",
		stats.RoomsCreated.Load(), time.Since(startTime))

	// start user sessions
	log.Println("phase 4: starting user sessions...")
	startTime = time.Now()
	successfulSessions := 0

	for i, user := range users {
		if user == nil {
			continue
		}

		if user.AccessToken == "" {
			log.Printf("skipping session %d (no access token for %s)", i, user.Email)
			continue
		}

		roomIndex := i / NumSessionsPerRoom
		if roomIndex >= len(rooms) || rooms[roomIndex] == 0 {
			log.Printf("skipping session %d (room doesn't exist)", i)
			continue
		}

		wg.Add(1)
		successfulSessions++

		if i%100 == 0 && i > 0 {
			time.Sleep(50 * time.Millisecond)
		}

		go func(i int, u *User) {
			defer wg.Done()
			roomID := rooms[i/NumSessionsPerRoom]
			stats.SessionsActive.Add(1)
			handleUserSession(ctx, u, roomID)
			stats.SessionsActive.Add(-1)
		}(i, user)
	}

	log.Printf("all sessions started in %v (successful: %d/%d)\n",
		time.Since(startTime), successfulSessions, totalUsers)
	log.Println("press ctrl+c to stop and cleanup...")

	wg.Wait()
	log.Println("=== Stress test complete ===")

	// shouldnt really happen, but maby i will implement a kick message for a client to close the connection
	deleteAllUsers(&usersMutex, allUsers)

	printFinalStats()
}

func deleteAllUsers(mutex *sync.RWMutex, users []*User) {
	mutex.RLock()
	userCount := len(users)
	mutex.RUnlock()

	if userCount == 0 {
		log.Println("no users to delete")
		return
	}

	log.Printf("deleting %d users...\n", userCount)
	startTime := time.Now()

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, MaxConcurrentHTTPRequests)

	mutex.RLock()
	defer mutex.RUnlock()

	for i, user := range users {
		if user == nil || user.AccessToken == "" {
			continue
		}

		wg.Add(1)
		semaphore <- struct{}{}

		go func(index int, u *User) {
			defer wg.Done()
			defer func() { <-semaphore }()

			if u.deleteAccount() {
				stats.UsersDeleted.Add(1)
				if index%50 == 0 {
					log.Printf("deleted user %d/%d", index+1, userCount)
				}
			}
		}(i, user)
	}

	wg.Wait()

	log.Printf("deleted %d users in %v\n", stats.UsersDeleted.Load(), time.Since(startTime))
}

func createUser(i int) *User {
	u := &User{
		Email:    fmt.Sprintf("perfuser%d@example.com", i),
		Password: "Password123#",
	}

	body, err := json.Marshal(map[string]string{
		"email":    u.Email,
		"password": u.Password,
	})
	if err != nil {
		log.Printf("failed to marshal user %d: %v", i, err)
		return nil
	}

	resp, err := httpClient.Post(
		"http://"+ServerAddress+":80/auth/register",
		"application/json",
		bytes.NewBuffer(body),
	)

	if err != nil {
		log.Printf("createUser %d failed: %v", i, err)
		return nil
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		log.Printf("createUser %d returned status %d: %s",
			i, resp.StatusCode, string(bodyBytes))
		return nil
	}

	return u
}

func (u *User) login() bool {
	if u == nil {
		return false
	}

	u.DeviceID, _ = gocql.RandomUUID()

	body, _ := json.Marshal(map[string]string{
		"email":     u.Email,
		"password":  u.Password,
		"device_id": u.DeviceID.String(),
	})

	resp, err := httpClient.Post(
		"http://"+ServerAddress+":80/auth/login",
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		log.Printf("login failed for %s: %v", u.Email, err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return false
	}

	var data struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)

	u.AccessToken = data.AccessToken
	u.RefreshToken = data.RefreshToken
	return true
}

func (u *User) createRoom() uint32 {
	if u == nil || u.AccessToken == "" {
		return 0
	}

	body, _ := json.Marshal(map[string]string{
		"roomname": "Perf Test Room by " + u.Email,
	})

	req, _ := http.NewRequest(
		"POST",
		"http://"+ServerAddress+":80/game/create_room",
		bytes.NewBuffer(body),
	)
	req.Header.Set("Authorization", "Bearer "+u.AccessToken)
	req.Header.Set("X-Device-ID", u.DeviceID.String())
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("createRoom failed for %s: %v", u.Email, err)
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		io.Copy(io.Discard, resp.Body)
		return 0
	}

	var data struct {
		RoomID uint32 `json:"roomId"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0
	}
	io.Copy(io.Discard, resp.Body)

	return data.RoomID
}

func (u *User) deleteAccount() bool {
	if u == nil || u.AccessToken == "" {
		return false
	}

	req, err := http.NewRequest(
		"POST",
		"http://"+ServerAddress+":80/auth/delete_account",
		nil,
	)
	if err != nil {
		log.Printf("failed to create delete request for %s: %v", u.Email, err)
		return false
	}

	req.Header.Set("Authorization", "Bearer "+u.AccessToken)
	req.Header.Set("X-Device-ID", u.DeviceID.String())
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("deleteAccount failed for %s: %v", u.Email, err)
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return false
	}

	return true
}

func handleUserSession(ctx context.Context, u *User, roomID uint32) {
	tcpConn, err := net.Dial("tcp", ServerAddress+":9000")
	if err != nil {
		log.Printf("[%s] TCP connection failed: %v", u.Email, err)
		stats.TotalErrors.Add(1)
		return
	}
	defer tcpConn.Close()

	udpAddr, err := net.ResolveUDPAddr("udp", ServerAddress+":9001")
	if err != nil {
		log.Printf("[%s] failed to resolve UDP address: %v", u.Email, err)
		stats.TotalErrors.Add(1)
		return
	}

	udpConn, err := net.Dial("udp", udpAddr.String())
	if err != nil {
		log.Printf("[%s] UDP connection failed: %v", u.Email, err)
		stats.TotalErrors.Add(1)
		return
	}
	defer udpConn.Close()

	helloPacket := protocol.Hello{
		RoomId:      roomID,
		DeviceID:    u.DeviceID,
		AccessToken: []byte(u.AccessToken),
	}
	helloData, err := helloPacket.Encode()
	if err != nil {
		log.Printf("[%s] Failed to encode hello packet: %v", u.Email, err)
		return
	}
	tcpConn.Write(helloData)

	time.Sleep(1 * time.Second)

	udpConn.Write(helloData) // connect via udp

	entities := make([]gocql.UUID, NumEntitysPerSession)
	for i := 0; i < NumEntitysPerSession; i++ { // create all entities for session
		randomUUID, _ := gocql.RandomUUID()
		newEntityPacket := protocol.NewEntity{
			EntityID:   randomUUID,
			EntityType: uint16(rand.Intn(5) + 1),
			CustomData: make([]byte, 10),
		}
		newEntityData, err := newEntityPacket.Encode()
		if err != nil {
			log.Printf("[%s] failed to encode new entity packet: %v", u.Email, err)
			return
		}
		tcpConn.Write(newEntityData)

		entities[i] = newEntityPacket.EntityID
	}

	ticker := time.NewTicker(PacketIntervalMs * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for i := 0; i < NumEntitysPerSession; i++ {
				movePacket := protocol.EntityMove{
					EntityID: entities[i],
				}
				moveData, err := movePacket.Encode()
				if err != nil {
					continue
				}
				udpConn.Write(moveData)
			}
		}
	}
}

func readMessage(reader *bufio.Reader) (uint8, []byte, error) { // stolen from server code :)
	magic := make([]byte, 3)
	_, err := io.ReadFull(reader, magic)
	if err != nil {
		return 0, nil, err
	}

	if string(magic) != protocol.MagicBytes {
		return 0, nil, errors.New("invalid magic bytes")
	}

	msgTypeBuf := make([]byte, 1)
	_, err = io.ReadFull(reader, msgTypeBuf)
	if err != nil {
		return 0, nil, err
	}
	msgType := msgTypeBuf[0]

	lengthBuf := make([]byte, 4)
	_, err = io.ReadFull(reader, lengthBuf)
	if err != nil {
		return 0, nil, err
	}
	length := binary.LittleEndian.Uint32(lengthBuf)

	payload := make([]byte, length)
	_, err = io.ReadFull(reader, payload)
	if err != nil {
		return 0, nil, err
	}

	return msgType, payload, nil
}

func reportStats(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			log.Printf("stats: users=%d, logins=%d, rooms=%d, active=%d, errors=%d", // there are no errors just happy mistakes!
				stats.UsersCreated.Load(),
				stats.UsersLoggedIn.Load(),
				stats.RoomsCreated.Load(),
				stats.SessionsActive.Load(),
				stats.TotalErrors.Load())
		}
	}
}

func printFinalStats() {
	log.Println("\n=== final statistics ===")
	log.Printf("users created:     %d", stats.UsersCreated.Load())
	log.Printf("users logged In:   %d", stats.UsersLoggedIn.Load())
	log.Printf("rooms created:     %d", stats.RoomsCreated.Load())
	log.Printf("users deleted:     %d", stats.UsersDeleted.Load())
	log.Printf("total errors:      %d", stats.TotalErrors.Load())
}
