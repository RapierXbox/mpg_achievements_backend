package handlers

import (
	"backend/internal/game"
	"encoding/json"

	"net/http"

	"github.com/gocql/gocql"
)

type GameHandler struct {
	gameManger *game.Manager
}

// NewGameHandler creates a new game handler
func NewGameHandler(gameManager *game.Manager) *GameHandler {
	return &GameHandler{
		gameManger: gameManager,
	}
}

func (h *GameHandler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(gocql.UUID)
	if !ok {
		respondError(w, "authentication required", http.StatusUnauthorized)
		return
	}

	var req struct {
		RoomName string `json:"roomname"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "invalid request format - "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.RoomName == "" || len(req.RoomName) > 100 {
		respondError(w, "invalid room name", http.StatusBadRequest)
		return
	}

	room, err := h.gameManger.CreateRoom(userID, req.RoomName)
	if err != nil {
		respondError(w, "failed to create room - "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"roomId": room.ID,
	})
}

func (h *GameHandler) GetRooms(w http.ResponseWriter, r *http.Request) {
	_, ok := r.Context().Value("userID").(gocql.UUID)
	if !ok {
		respondError(w, "authentication required", http.StatusUnauthorized)
		return
	}

	rooms := h.gameManger.GetAllRooms()
	roomSummaries := make([]map[string]interface{}, 0, len(rooms))

	for _, room := range rooms {
		room.GetMutex().RLock()
		numSessions := len(room.Sessions)
		room.GetMutex().RUnlock()

		roomSummaries = append(roomSummaries, map[string]interface{}{
			"roomId":   room.ID,
			"roomName": room.Name,
			"numUsers": numSessions,
		})
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"rooms": roomSummaries,
	})
}

func (h *GameHandler) DeleteRoom(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(gocql.UUID)
	if !ok {
		respondError(w, "authentication required", http.StatusUnauthorized)
		return
	}

	var req struct {
		RoomID uint32 `json:"roomid"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "invalid request format - "+err.Error(), http.StatusBadRequest)
		return
	}

	room, err := h.gameManger.GetRoom(req.RoomID)
	if err != nil {
		respondError(w, "failed to get room - "+err.Error(), http.StatusInternalServerError)
		return
	}

	if room.OwnerID != userID {
		respondError(w, "user is not the owner of the room", http.StatusForbidden)
		return
	}

	err = h.gameManger.DeleteRoom(req.RoomID)
	if err != nil {
		respondError(w, "failed to delete room - "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
