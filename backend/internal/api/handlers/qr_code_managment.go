package handlers

import (
	"backend/internal/models"
	"backend/internal/repository"
	"backend/internal/service"
	"encoding/json"
	"strconv"
	"time"

	"net/http"

	"github.com/gocql/gocql"
)

// Debughandler manages debug endpoints
type QRCodeManagementHandler struct {
	qr_service   *service.QRService
	account_repo *repository.AccountRepository
}

// NewAuthHandler creates a new auth handler
func NewQRCodeManagementHandler(qr_service *service.QRService, account_repo *repository.AccountRepository) *QRCodeManagementHandler {
	return &QRCodeManagementHandler{
		qr_service:   qr_service,
		account_repo: account_repo,
	}
}

func (h *QRCodeManagementHandler) ValidateAdmin(w http.ResponseWriter, r *http.Request) bool {
	// get user id from context
	user_id, ok := r.Context().Value("userID").(gocql.UUID)
	if !ok {
		respondError(w, "authentication required", http.StatusUnauthorized)
		return true
	}

	user_account, err := h.account_repo.GetAccountByID(user_id)
	if err != nil || user_account == nil {
		respondError(w, "could not find user account - "+err.Error(), http.StatusUnauthorized)
		return true
	}

	if !user_account.Admin {
		respondError(w, "admin privileges required", http.StatusForbidden)
		return true
	}

	return false
}

// -------------------------------------- HANDLERS -----------------------------------------------

func (h *QRCodeManagementHandler) AddQRCode(w http.ResponseWriter, r *http.Request) {
	blocked := h.ValidateAdmin(w, r)
	if blocked {
		return
	}

	var req struct {
		ActionID   string `json:"action_id"`
		QrCodeType int    `json:"qr_code_type"`
		MaxUsages  int    `json:"max_usages"`  // 0 = unlimited
		ExpireMins int    `json:"expire_mins"` // 0 = never expires
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request format", http.StatusBadRequest)
		return
	}

	action_id, err := gocql.ParseUUID(req.ActionID)
	if err != nil {
		respondError(w, "invalid action_id", http.StatusBadRequest)
		return
	}

	qr_action, err := h.qr_service.GetQRActionById(action_id)
	if err != nil || qr_action == nil {
		respondError(w, "could not find qr action for given action_id", http.StatusBadRequest)
		return
	}

	qr_code, err := h.qr_service.AddQRCode(action_id, models.QRCodeUsageType(req.QrCodeType), req.MaxUsages, time.Now().UTC().Add(time.Duration(req.ExpireMins)*time.Minute))
	if err != nil {
		respondError(w, "could not add qr code - "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"qr_code_id": qr_code.ID,
	})
}

func (h *QRCodeManagementHandler) AddQRAction(w http.ResponseWriter, r *http.Request) {
	blocked := h.ValidateAdmin(w, r)
	if blocked {
		return
	}

	var req struct {
		ActionJson string `json:"action_json"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request format", http.StatusBadRequest)
		return
	}

	qr_action, err := h.qr_service.AddQRAction(req.ActionJson)
	if err != nil {
		respondError(w, "could not add qr action - "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"qr_action_id": qr_action.ID,
	})
}

func (h *QRCodeManagementHandler) DeleteQRCode(w http.ResponseWriter, r *http.Request) {
	blocked := h.ValidateAdmin(w, r)
	if blocked {
		return
	}

	var req struct {
		QrCodeId string `json:"qr_code_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request format", http.StatusBadRequest)
		return
	}

	qr_code_id, err := gocql.ParseUUID(req.QrCodeId)
	if err != nil {
		respondError(w, "invalid qr_code_id - "+err.Error(), http.StatusBadRequest)
		return
	}

	err = h.qr_service.DeleteQRCode(qr_code_id)
	if err != nil {
		respondError(w, "could not delete qr code - "+err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "qr code deleted",
	})
}

func (h *QRCodeManagementHandler) DeleteQRAction(w http.ResponseWriter, r *http.Request) {
	blocked := h.ValidateAdmin(w, r)
	if blocked {
		return
	}

	var req struct {
		QrActionId string `json:"qr_action_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request format", http.StatusBadRequest)
		return
	}

	qr_action_id, err := gocql.ParseUUID(req.QrActionId)
	if err != nil {
		respondError(w, "invalid qr_action_id - "+err.Error(), http.StatusBadRequest)
		return
	}

	err = h.qr_service.DeleteQRAction(qr_action_id)
	if err != nil {
		respondError(w, "could not delete qr action - "+err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "qr action deleted",
	})
}

func (h *QRCodeManagementHandler) GetAllQRActions(w http.ResponseWriter, r *http.Request) {
	blocked := h.ValidateAdmin(w, r)
	if blocked {
		return
	}

	max_count, err := strconv.Atoi(r.URL.Query().Get("count"))
	if err != nil {
		respondError(w, "invalid count", http.StatusBadRequest)
		return
	}

	qr_actions, err := h.qr_service.GetAllQRActions(max_count)
	if err != nil {
		respondError(w, "could not get qr actions - "+err.Error(), http.StatusInternalServerError)
		return
	}

	json_bytes, err := json.Marshal(qr_actions)
	if err != nil {
		respondError(w, "could not marshal qr actions - "+err.Error(), http.StatusInternalServerError)
		return
	}

	var result interface{}
	err = json.Unmarshal(json_bytes, &result)
	if err != nil {
		respondError(w, "could not unmarshal qr actions - "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (h *QRCodeManagementHandler) GetAllQRCodes(w http.ResponseWriter, r *http.Request) {
	blocked := h.ValidateAdmin(w, r)
	if blocked {
		return
	}

	max_count, err := strconv.Atoi(r.URL.Query().Get("count"))
	if err != nil {
		respondError(w, "invalid count", http.StatusBadRequest)
		return
	}

	qr_codes, err := h.qr_service.GetAllQRCodes(max_count)
	if err != nil {
		respondError(w, "could not get qr codes - "+err.Error(), http.StatusInternalServerError)
		return
	}

	json_bytes, err := json.Marshal(qr_codes)
	if err != nil {
		respondError(w, "could not marshal qr codes - "+err.Error(), http.StatusInternalServerError)
		return
	}

	var result interface{}
	err = json.Unmarshal(json_bytes, &result)
	if err != nil {
		respondError(w, "could not unmarshal qr codes - "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, result)
}
