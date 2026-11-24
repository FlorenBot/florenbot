package handlers

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"strings"

	helpers "florenbot/backend/engine/helpers"
	"florenbot/services"
)

type ReputateHandler struct {
	HTTP *services.HTTPService
}

func NewReputateHandler(httpService *services.HTTPService) *ReputateHandler {
	return &ReputateHandler{
		HTTP: httpService,
	}
}

func (h *ReputateHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/reputation/thanks", h.Thanks)
	mux.HandleFunc("POST /api/v1/reputation/change", h.ChangeReputation)
}

type thanksRequest struct {
	TargetUserID uint64 `json:"target_user_id"`
}

type changeReputationRequest struct {
	ActorUserID  uint64 `json:"actor_user_id"`
	TargetUserID uint64 `json:"target_user_id"`
	TargetName   string `json:"target_name,omitempty"`
	Action       string `json:"action"` // "полож" | "отриц"
	Amount       int    `json:"amount"`
}

type reputateAPIResponse struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

func writeReputateJSON(w http.ResponseWriter, status int, payload reputateAPIResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// Thanks — POST /api/v1/reputation/thanks
func (h *ReputateHandler) Thanks(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req thanksRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeReputateJSON(w, http.StatusBadRequest, reputateAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}
	if req.TargetUserID == 0 {
		writeReputateJSON(w, http.StatusBadRequest, reputateAPIResponse{OK: false, Code: "INVALID_TARGET_USER", Error: "target_user_id is required"})
		return
	}

	debugType := isDebugEnabled()

	if _, err := helpers.GetUserByID(req.TargetUserID); err != nil {
		if debugType {
			log.Printf("Ошибка получения пользователя: %v", err)
		}
		writeReputateJSON(w, http.StatusNotFound, reputateAPIResponse{OK: false, Code: "USER_NOT_FOUND", Error: "target user not found"})
		return
	}

	currentReputate, err := helpers.GetReputation(req.TargetUserID)
	if err != nil {
		if debugType {
			log.Printf("Ошибка получения репутации: %v", err)
		}
		writeReputateJSON(w, http.StatusInternalServerError, reputateAPIResponse{OK: false, Code: "GET_REP_FAILED", Error: "failed to get reputation"})
		return
	}

	delta := rand.Intn(100)
	totalReputate := currentReputate + delta
	if debugType {
		log.Printf("Репутация пользователя %d увеличена на %d: %d", req.TargetUserID, delta, totalReputate)
	}

	if err := helpers.UpdatePositiveReputation(req.TargetUserID, totalReputate); err != nil {
		writeReputateJSON(w, http.StatusInternalServerError, reputateAPIResponse{OK: false, Code: "UPDATE_REP_FAILED", Error: "failed to update reputation"})
		return
	}

	writeReputateJSON(w, http.StatusOK, reputateAPIResponse{
		OK: true,
		Data: map[string]any{
			"target_user_id":   req.TargetUserID,
			"added":            delta,
			"total_reputation": totalReputate,
		},
	})
}

// ChangeReputation — POST /api/v1/reputation/change
func (h *ReputateHandler) ChangeReputation(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req changeReputationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeReputateJSON(w, http.StatusBadRequest, reputateAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}

	debugType := isDebugEnabled()
	action := strings.TrimSpace(req.Action)

	if req.ActorUserID == 0 || req.TargetUserID == 0 {
		writeReputateJSON(w, http.StatusBadRequest, reputateAPIResponse{OK: false, Code: "INVALID_USER", Error: "actor_user_id and target_user_id are required"})
		return
	}
	if req.Amount <= 0 {
		writeReputateJSON(w, http.StatusBadRequest, reputateAPIResponse{OK: false, Code: "INVALID_AMOUNT", Error: "amount must be positive"})
		return
	}
	if req.TargetUserID == req.ActorUserID {
		if debugType {
			log.Printf("Пользователь %d пытается изменить репутацию самому себе", req.ActorUserID)
		}
		writeReputateJSON(w, http.StatusBadRequest, reputateAPIResponse{OK: false, Code: "SELF_REPUTATION", Error: "cannot change own reputation"})
		return
	}
	if action != "полож" && action != "отриц" {
		writeReputateJSON(w, http.StatusBadRequest, reputateAPIResponse{OK: false, Code: "INVALID_ACTION", Error: "invalid reputation action"})
		return
	}

	if _, err := helpers.GetUserByID(req.ActorUserID); err != nil {
		writeReputateJSON(w, http.StatusUnauthorized, reputateAPIResponse{OK: false, Code: "UNAUTHORIZED", Error: "actor user not found"})
		return
	}

	if _, err := helpers.GetUserByID(req.TargetUserID); err != nil {
		if debugType {
			log.Printf("Ошибка получения пользователя: %v", err)
		}
		writeReputateJSON(w, http.StatusNotFound, reputateAPIResponse{OK: false, Code: "TARGET_NOT_FOUND", Error: "target user not found"})
		return
	}

	currentReputate, err := helpers.GetReputation(req.TargetUserID)
	if err != nil {
		if debugType {
			log.Printf("Ошибка получения репутации: %v", err)
		}
		writeReputateJSON(w, http.StatusInternalServerError, reputateAPIResponse{OK: false, Code: "GET_REP_FAILED", Error: "failed to get reputation"})
		return
	}

	switch action {
	case "отриц":
		if currentReputate <= 0 {
			writeReputateJSON(w, http.StatusConflict, reputateAPIResponse{OK: false, Code: "NO_REPUTATION", Error: "user has no reputation to decrease"})
			return
		}
		totalReputate := currentReputate - req.Amount
		if err := helpers.UpdateNegativeReputation(req.TargetUserID, totalReputate); err != nil {
			writeReputateJSON(w, http.StatusInternalServerError, reputateAPIResponse{OK: false, Code: "UPDATE_REP_FAILED", Error: "failed to update reputation"})
			return
		}
		writeReputateJSON(w, http.StatusOK, reputateAPIResponse{
			OK: true,
			Data: map[string]any{
				"action":           "отриц",
				"target_user_id":   req.TargetUserID,
				"target_name":      req.TargetName,
				"amount":           req.Amount,
				"total_reputation": totalReputate,
			},
		})

	case "полож":
		totalReputate := currentReputate + req.Amount
		if err := helpers.UpdatePositiveReputation(req.TargetUserID, totalReputate); err != nil {
			writeReputateJSON(w, http.StatusInternalServerError, reputateAPIResponse{OK: false, Code: "UPDATE_REP_FAILED", Error: "failed to update reputation"})
			return
		}
		writeReputateJSON(w, http.StatusOK, reputateAPIResponse{
			OK: true,
			Data: map[string]any{
				"action":           "полож",
				"target_user_id":   req.TargetUserID,
				"target_name":      req.TargetName,
				"amount":           req.Amount,
				"total_reputation": totalReputate,
			},
		})
	}
}