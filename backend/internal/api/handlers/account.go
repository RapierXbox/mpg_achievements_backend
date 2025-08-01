package handlers

import (
	"backend/internal/repository"
	"backend/internal/service"

	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

// AccountHandler handles account-related HTTP requests
type AccountHandler struct {
	accountService *service.AccountService
}

// NewAccountHandler creates a new account handler
func NewAccountHandler(accountService *service.AccountService) *AccountHandler {
	return &AccountHandler{
		accountService: accountService,
	}
}

// register handles user registration
func (h *AccountHandler) Register(w http.ResponseWriter, r *http.Request) {
	// parse request
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "invalid request format", http.StatusBadRequest)
		return
	}

	// create account
	account, err := h.accountService.RegisterAccount(req.Email, req.Password)
	if err != nil {
		status := http.StatusInternalServerError
		if err == repository.ErrEmailExists || err.Error() == "invalid email format" {
			status = http.StatusBadRequest
		}
		respondError(w, err.Error(), status)
		return
	}

	// response without sensitive data
	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         account.ID.String(),
		"email":      account.Email,
		"created_at": account.CreatedAt,
	})
}

// ChangePassword handles password updates
func (h *AccountHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	// get user ID from context
	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		respondError(w, "authentication required", http.StatusUnauthorized)
		return
	}

	// parse request
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "invalid request", http.StatusBadRequest)
		return
	}

	// convert to UUID
	uuid, err := uuid.Parse(userID)
	if err != nil {
		respondError(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	// update password
	if err := h.accountService.ChangePassword(uuid, req.OldPassword, req.NewPassword); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "invalid current password" {
			status = http.StatusUnauthorized
		}
		respondError(w, err.Error(), status)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "password updated"})
}

// Delete handles account deletion
func (h *AccountHandler) Delete(w http.ResponseWriter, r *http.Request) {
	// get user ID from context
	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		respondError(w, "authentication required", http.StatusUnauthorized)
		return
	}

	// convert to UUID
	uuid, err := uuid.Parse(userID)
	if err != nil {
		respondError(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	// delete account
	if err := h.accountService.DeleteAccount(uuid); err != nil {
		status := http.StatusInternalServerError
		respondError(w, err.Error(), status)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "account deleted"})
}
