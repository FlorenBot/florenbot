package routes

import (
	"encoding/json"
	"log"
	"net/http"

	cache "florenbot/backend/engine/cache"
	helpers "florenbot/backend/engine/helpers"
	"florenbot/services"
)

const minBonesBet = 30.0

type BonesHandler struct {
	HTTP *services.HTTPService
}

func NewBonesHandler(httpService *services.HTTPService) *BonesHandler {
	return &BonesHandler{
		HTTP: httpService,
	}
}

func (h *BonesHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/games/bones/check", h.CheckBet)
	mux.HandleFunc("POST /api/v1/games/bones/resolve", h.ResolveRound)
}

type bonesCheckRequest struct {
	UserID   uint64  `json:"user_id"`
	Username string  `json:"username,omitempty"`
	Bet      float64 `json:"bet"`
}

type bonesResolveRequest struct {
	UserID      uint64  `json:"user_id"`
	Bet         float64 `json:"bet"`
	PlayerValue int     `json:"player_value"`
	BotValue    int     `json:"bot_value"`
}

type bonesAPIResponse struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

func writeBonesJSON(w http.ResponseWriter, status int, payload bonesAPIResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// CheckBet — POST /api/v1/games/bones/check
func (h *BonesHandler) CheckBet(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req bonesCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBonesJSON(w, http.StatusBadRequest, bonesAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}
	if req.UserID == 0 {
		writeBonesJSON(w, http.StatusBadRequest, bonesAPIResponse{OK: false, Code: "INVALID_USER", Error: "user_id is required"})
		return
	}
	if req.Bet <= 0 {
		writeBonesJSON(w, http.StatusBadRequest, bonesAPIResponse{OK: false, Code: "INVALID_BET", Error: "bet must be positive"})
		return
	}

	balance, err := cache.GetBalance(req.UserID, req.Username)
	if err != nil {
		log.Printf("Ошибка проверки баланса в /bones: %v", err)
		writeBonesJSON(w, http.StatusInternalServerError, bonesAPIResponse{OK: false, Code: "BALANCE_ERROR", Error: "failed to check balance"})
		return
	}

	if req.Bet > balance {
		writeBonesJSON(w, http.StatusConflict, bonesAPIResponse{
			OK:    false,
			Code:  "INSUFFICIENT_FUNDS",
			Error: "insufficient balance",
			Data: map[string]any{
				"balance": balance,
			},
		})
		return
	}

	if req.Bet < minBonesBet {
		writeBonesJSON(w, http.StatusBadRequest, bonesAPIResponse{
			OK:    false,
			Code:  "MIN_BET",
			Error: "bet is below minimum",
			Data: map[string]any{
				"min_bet": minBonesBet,
			},
		})
		return
	}

	writeBonesJSON(w, http.StatusOK, bonesAPIResponse{
		OK: true,
		Data: map[string]any{
			"balance": balance,
			"bet":     req.Bet,
		},
	})
}

// ResolveRound — POST /api/v1/games/bones/resolve
func (h *BonesHandler) ResolveRound(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req bonesResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBonesJSON(w, http.StatusBadRequest, bonesAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}
	if req.UserID == 0 || req.Bet <= 0 {
		writeBonesJSON(w, http.StatusBadRequest, bonesAPIResponse{OK: false, Code: "INVALID_PARAMS", Error: "user_id and positive bet are required"})
		return
	}

	switch {
	case req.PlayerValue > req.BotValue:
		if err := cache.ChangeBalance(req.UserID, req.Bet); err != nil {
			log.Printf("Ошибка изменения баланса: %v", err)
			writeBonesJSON(w, http.StatusInternalServerError, bonesAPIResponse{OK: false, Code: "BALANCE_CHANGE_FAILED", Error: "failed to change balance"})
			return
		}
		if err := helpers.AddUserToLosses(req.UserID); err != nil {
			log.Printf("Ошибка изменения рекордов: %v", err)
			writeBonesJSON(w, http.StatusInternalServerError, bonesAPIResponse{OK: false, Code: "STATS_UPDATE_FAILED", Error: "failed to update stats"})
			return
		}
		writeBonesJSON(w, http.StatusOK, bonesAPIResponse{
			OK: true,
			Data: map[string]any{
				"outcome": "win",
				"delta":   req.Bet,
			},
		})

	case req.PlayerValue < req.BotValue:
		if err := cache.ChangeBalance(req.UserID, -req.Bet); err != nil {
			log.Printf("Ошибка изменения баланса: %v", err)
			writeBonesJSON(w, http.StatusInternalServerError, bonesAPIResponse{OK: false, Code: "BALANCE_CHANGE_FAILED", Error: "failed to change balance"})
			return
		}
		if err := helpers.AddUserToLosses(req.UserID); err != nil {
			log.Printf("Ошибка изменения рекордов: %v", err)
			writeBonesJSON(w, http.StatusInternalServerError, bonesAPIResponse{OK: false, Code: "STATS_UPDATE_FAILED", Error: "failed to update stats"})
			return
		}
		writeBonesJSON(w, http.StatusOK, bonesAPIResponse{
			OK: true,
			Data: map[string]any{
				"outcome": "lose",
				"delta":   -req.Bet,
			},
		})

	default:
		writeBonesJSON(w, http.StatusOK, bonesAPIResponse{
			OK: true,
			Data: map[string]any{
				"outcome": "draw",
				"delta":   0.0,
			},
		})
	}
}