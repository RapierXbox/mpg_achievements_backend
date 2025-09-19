package handlers

import (
	"backend/internal/service"
	"encoding/json"

	"net/http"

	"github.com/gocql/gocql"
)

// Debughandler manages debug endpoints
type QRCodeHandler struct {
	qr_service *service.QRService
}

// NewAuthHandler creates a new auth handler
func NewQRCodeHandler(qr_service *service.QRService) *QRCodeHandler {
	return &QRCodeHandler{
		qr_service: qr_service,
	}
}

func (h *QRCodeHandler) GetQRAction(w http.ResponseWriter, r *http.Request) {
	// get user id from context
	user_id, ok := r.Context().Value("userID").(gocql.UUID)
	if !ok {
		respondError(w, "authentication required", http.StatusUnauthorized)
		return
	}

	// qr_code_id from query params
	qr_code_id, err := gocql.ParseUUID(r.URL.Query().Get("qr_code_id"))
	if err != nil {
		respondError(w, "invalid qr_code_id - "+err.Error(), http.StatusBadRequest)
		return
	}

	qr_code_action, err := h.qr_service.GetActionJsonFromQRCodeId(qr_code_id, user_id)
	if err != nil {
		respondError(w, "could not get qr code action - "+err.Error(), http.StatusBadRequest)
		return
	}

	var qr_action_json map[string]interface{}
	err = json.Unmarshal([]byte(qr_code_action), &qr_action_json)
	if err != nil {
		respondError(w, "could not parse qr action json - "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, qr_action_json)
}
