package handlers

import (
	"backend/internal/service"
	"backend/pkg/config"
	"backend/pkg/utils"

	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// AuthHandler manages authentication endpoints
type AuthHandler struct {
	accountService *service.AccountService
	sessionService *service.SessionService
	cfg            *config.Config
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(accountService *service.AccountService, sessionService *service.SessionService, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		accountService: accountService,
		sessionService: sessionService,
		cfg:            cfg,
	}
}

// Login authenticates users and creates sessions
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// parse request body
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		DeviceID string `json:"device_id"` // from client device
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request format", http.StatusBadRequest)
		return
	}

	// authenticate credentials
	user, err := h.accountService.Authenticate(req.Email, req.Password)
	if err != nil {
		respondError(w, "authentication failed - "+err.Error(), http.StatusUnauthorized)
		return
	}

	// generate token pair
	accessToken, refreshToken, err := utils.GenerateTokenPair(
		user.ID.String(),
		[]byte(h.cfg.JWTSecret),
		time.Duration(h.cfg.AccessTokenTTL)*time.Minute,
		time.Duration(h.cfg.RefreshTokenTTL)*24*time.Hour,
	)
	if err != nil {
		respondError(w, "token generation failed - "+err.Error(), http.StatusInternalServerError)
		return
	}

	// create persistent session
	if err := h.sessionService.CreatePermanentSession(
		user.ID.String(),
		req.DeviceID,
		refreshToken,
	); err != nil {
		respondError(w, "session creation failed", http.StatusInternalServerError)
		return
	}

	// successful response
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"expires_in":    h.cfg.AccessTokenTTL * 60,
		"user_id":       user.ID.String(),
	})
}

// SilentLogin refreshes tokens using existing session
func (h *AuthHandler) SilentLogin(w http.ResponseWriter, r *http.Request) {
	// extract device ID from header
	deviceID := r.Header.Get("X-Device-ID")
	if deviceID == "" {
		respondError(w, "device ID required", http.StatusBadRequest)
		return
	}

	// extract refresh token from authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		respondError(w, "authorization required", http.StatusUnauthorized)
		return
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")

	// extract user ID from token
	claims, err := utils.ParseToken(token, []byte(h.cfg.JWTSecret))
	if err != nil {
		respondError(w, "Invalid token", http.StatusForbidden)
		return
	}
	userID := claims["sub"].(string)

	// validate session and device binding
	valid, err := h.sessionService.ValidateSession(userID, deviceID, token)
	if err != nil || !valid {
		respondError(w, "Invalid session", http.StatusUnauthorized)
		return
	}

	// generate new token pair
	newAccessToken, newRefreshToken, err := utils.GenerateTokenPair(
		userID,
		[]byte(h.cfg.JWTSecret),
		time.Duration(h.cfg.AccessTokenTTL)*time.Minute,
		time.Duration(h.cfg.RefreshTokenTTL)*24*time.Hour,
	)
	if err != nil {
		respondError(w, "token generation failed", http.StatusInternalServerError)
		return
	}

	// rotate refresh token for security
	if err := h.sessionService.RotateSession(
		userID,
		deviceID,
		token,
		newRefreshToken,
	); err != nil {
		respondError(w, "session update failed - "+err.Error(), http.StatusInternalServerError)
		return
	}

	// return new tokens
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  newAccessToken,
		"refresh_token": newRefreshToken,
		"expires_in":    h.cfg.AccessTokenTTL * 60,
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Get user id from context
	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		respondError(w, "authentication required", http.StatusUnauthorized)
		return
	}

	// get device id from header
	deviceID := r.Header.Get("X-Device-ID")
	if deviceID == "" {
		respondError(w, "device ID required", http.StatusBadRequest)
		return
	}

	// delete session from database
	if err := h.sessionService.DeleteSession(userID, deviceID); err != nil {
		respondError(w, "logout failed - "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// helper to send json responses
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// helper to send error responses
func respondError(w http.ResponseWriter, message string, status int) {
	respondJSON(w, status, map[string]string{"error": message})
}
