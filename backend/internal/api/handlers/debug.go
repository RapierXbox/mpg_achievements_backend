package handlers

import (
	"backend/pkg/config"

	"net/http"
)

// Debughandler manages debug endpoints
type DebugHandler struct {
	cfg *config.Config
}

// NewAuthHandler creates a new auth handler
func NewDebugHandler(cfg *config.Config) *DebugHandler {
	return &DebugHandler{
		cfg: cfg,
	}
}

// simple endpoint to test auth
func (h *DebugHandler) AuthDebug(w http.ResponseWriter, r *http.Request) {
	// get user id from context
	_, ok := r.Context().Value("userID").(string)
	if !ok {
		respondError(w, "authentication required", http.StatusUnauthorized)
		return
	}

	// successful response
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"debug": "success",
	})
}
