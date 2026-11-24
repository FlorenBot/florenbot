package routes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	cache "florenbot/backend/engine/cache"
	helpers "florenbot/backend/engine/helpers"
	helpers_user "florenbot/bot/helpers"
	"florenbot/services"
)

const minCasinoBet = 50

type CasinoHandler struct {
	HTTP *services.HTTPService
}

func NewCasinoHandler(httpService *services.HTTPService) *CasinoHandler {
	return &CasinoHandler{
		HTTP: httpService,
	}
}

func (h *CasinoHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/casino/info", h.Info)
	mux.HandleFunc("POST /api/v1/casino/slots", h.PlaySlots)
	mux.HandleFunc("POST /api/v1/casino/roulette", h.PlayRoulette)
}

type casinoRequest struct {
	UserID    uint64 `json:"user_id"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	Bet       int    `json:"bet"`
	Choice    string `json:"choice,omitempty"` // для рулетки: "красное", "черное", "зеленое"
}

type casinoResponse struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

func writeCasinoJSON(w http.ResponseWriter, status int, payload casinoResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func isDebugEnabled() bool {
	val, exists := os.LookupEnv("DEBUG")
	if !exists {
		return false
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return false
	}
	return b
}

// ensureUserAndBalance проверяет пользователя (при необходимости создаёт),
// проверяет минимальную ставку, достаточность баланса и rate limit.
func (h *CasinoHandler) ensureUserAndBalance(
	ctx context.Context,
	userID uint64,
	username string,
	firstName string,
	bet int,
) (float64, int, string, error) {
	debugType := isDebugEnabled()

	if userID == 0 {
		return 0, http.StatusBadRequest, "INVALID_USER", errors.New("field 'user_id' is required")
	}
	if bet <= 0 {
		return 0, http.StatusBadRequest, "INVALID_BET", errors.New("bet must be a positive number")
	}
	if bet < minCasinoBet {
		return 0, http.StatusBadRequest, "MIN_BET", fmt.Errorf("minimum bet is %d coins", minCasinoBet)
	}

	userProfile, err := helpers.GetUserByID(userID)
	if err != nil {
		return 0, http.StatusUnauthorized, "UNAUTHORIZED", errors.New("user is not authorized")
	}

	if userProfile.ID == 0 {
		if debugType {
			log.Printf("Пользователь %s создается...", firstName)
		}
		userProfile, err = helpers.CreateUser(userID, username, firstName)
		if err != nil {
			if debugType {
				log.Printf("Ошибка при создании пользователя: %v", err)
			}
			return 0, http.StatusInternalServerError, "USER_CREATE_FAILED", errors.New("failed to create user")
		}
		if debugType {
			log.Printf("Пользователь %s создан: %v", firstName, userProfile)
		}
	} else if debugType {
		log.Printf("Пользователь %s уже существует", firstName)
	}

	balance, err := cache.GetBalance(userID, username)
	if err != nil {
		if debugType {
			log.Printf("Ошибка получения баланса: %v", err)
		}
		return 0, http.StatusInternalServerError, "BALANCE_ERROR", errors.New("failed to check balance")
	}

	if float64(bet) > balance {
		return balance, http.StatusConflict, "INSUFFICIENT_FUNDS", fmt.Errorf("insufficient coins, balance: %.2f", balance)
	}

	allowed, err := helpers_user.CheckRateLimit(ctx, cache.RDB, userID, 5, 30*time.Second)
	if debugType {
		log.Printf("RateLimit Allowed: %v, Error: %v", allowed, err)
	}
	if err != nil {
		log.Printf("Ошибка проверки лимита: %v", err)
		return balance, http.StatusInternalServerError, "RATE_LIMIT_ERROR", errors.New("rate limit check failed")
	}
	if !allowed {
		return balance, http.StatusTooManyRequests, "RATE_LIMITED", errors.New("too many requests, retry after 30 seconds")
	}

	return balance, http.StatusOK, "", nil
}

// Info возвращает параметры игр казино.
func (h *CasinoHandler) Info(w http.ResponseWriter, r *http.Request) {
	writeCasinoJSON(w, http.StatusOK, casinoResponse{
		OK: true,
		Data: map[string]any{
			"min_bet":          minCasinoBet,
			"rate_limit":       "5 requests per 30s",
			"slots_symbols":    []string{"🍒", "🍋", "🎰", "💎"},
			"roulette_colors":  []string{"красное", "черное", "зеленое"},
			"roulette_numbers": "0..36",
		},
	})
}

// PlaySlots обрабатывает POST /api/v1/casino/slots (аналог HandleCasino).
func (h *CasinoHandler) PlaySlots(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req casinoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeCasinoJSON(w, http.StatusBadRequest, casinoResponse{
			OK:    false,
			Code:  "INVALID_JSON",
			Error: "invalid JSON body",
		})
		return
	}

	_, status, code, err := h.ensureUserAndBalance(r.Context(), req.UserID, req.Username, req.FirstName, req.Bet)
	if err != nil {
		writeCasinoJSON(w, status, casinoResponse{
			OK:    false,
			Code:  code,
			Error: err.Error(),
		})
		return
	}

	elements := []string{"🍒", "🍋", "🎰", "💎"}
	res1 := elements[rand.Intn(len(elements))]
	res2 := elements[rand.Intn(len(elements))]
	res3 := elements[rand.Intn(len(elements))]

	var (
		outcome      string
		balanceDelta float64
		displayWin   int
	)

	if res1 == res2 && res2 == res3 {
		outcome = "jackpot"
		winAmount := req.Bet * 5
		balanceDelta = float64(winAmount)
		displayWin = winAmount

		if err := cache.ChangeBalance(req.UserID, balanceDelta); err != nil {
			log.Printf("Ошибка изменения баланса: %v", err)
			writeCasinoJSON(w, http.StatusInternalServerError, casinoResponse{
				OK: false, Code: "BALANCE_UPDATE_FAILED", Error: "failed to update balance",
			})
			return
		}
		if err := helpers.AddUserToWins(req.UserID); err != nil {
			log.Printf("Ошибка изменения рекордов: %v", err)
			writeCasinoJSON(w, http.StatusInternalServerError, casinoResponse{
				OK: false, Code: "RECORDS_UPDATE_FAILED", Error: "failed to update win records",
			})
			return
		}
	} else if res1 == res2 || res2 == res3 || res1 == res3 {
		outcome = "win"
		winAmount := req.Bet * 1
		balanceDelta = float64(winAmount)
		displayWin = req.Bet * 2

		if err := cache.ChangeBalance(req.UserID, balanceDelta); err != nil {
			log.Printf("Ошибка изменения баланса: %v", err)
			writeCasinoJSON(w, http.StatusInternalServerError, casinoResponse{
				OK: false, Code: "BALANCE_UPDATE_FAILED", Error: "failed to update balance",
			})
			return
		}
		if err := helpers.AddUserToWins(req.UserID); err != nil {
			log.Printf("Ошибка изменения рекордов: %v", err)
			writeCasinoJSON(w, http.StatusInternalServerError, casinoResponse{
				OK: false, Code: "RECORDS_UPDATE_FAILED", Error: "failed to update win records",
			})
			return
		}
	} else {
		outcome = "lose"
		balanceDelta = float64(-req.Bet)
		displayWin = -req.Bet

		if err := cache.ChangeBalance(req.UserID, balanceDelta); err != nil {
			log.Printf("Ошибка изменения баланса: %v", err)
			writeCasinoJSON(w, http.StatusInternalServerError, casinoResponse{
				OK: false, Code: "BALANCE_UPDATE_FAILED", Error: "failed to update balance",
			})
			return
		}
		if err := helpers.AddUserToLosses(req.UserID); err != nil {
			log.Printf("Ошибка изменения рекордов: %v", err)
			writeCasinoJSON(w, http.StatusInternalServerError, casinoResponse{
				OK: false, Code: "RECORDS_UPDATE_FAILED", Error: "failed to update loss records",
			})
			return
		}
	}

	newBalance, err := cache.GetBalance(req.UserID, req.Username)
	if err != nil {
		newBalance = 0
	}

	writeCasinoJSON(w, http.StatusOK, casinoResponse{
		OK: true,
		Data: map[string]any{
			"reels":         []string{res1, res2, res3},
			"outcome":       outcome,
			"bet":           req.Bet,
			"balance_delta": balanceDelta,
			"display_win":   displayWin,
			"balance":       newBalance,
		},
	})
}

// PlayRoulette обрабатывает POST /api/v1/casino/roulette (аналог HandleRoulette).
func (h *CasinoHandler) PlayRoulette(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req casinoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeCasinoJSON(w, http.StatusBadRequest, casinoResponse{
			OK:    false,
			Code:  "INVALID_JSON",
			Error: "invalid JSON body",
		})
		return
	}

	choice := strings.ToLower(strings.TrimSpace(req.Choice))
	if choice != "красное" && choice != "черное" && choice != "зеленое" {
		writeCasinoJSON(w, http.StatusBadRequest, casinoResponse{
			OK:    false,
			Code:  "INVALID_CHOICE",
			Error: "choice must be one of: красное, черное, зеленое",
		})
		return
	}

	_, status, code, err := h.ensureUserAndBalance(r.Context(), req.UserID, req.Username, req.FirstName, req.Bet)
	if err != nil {
		writeCasinoJSON(w, status, casinoResponse{
			OK:    false,
			Code:  code,
			Error: err.Error(),
		})
		return
	}

	spin := rand.Intn(37)
	var landedColor string
	switch {
	case spin == 0:
		landedColor = "зеленое"
	case spin%2 == 0:
		landedColor = "красное"
	default:
		landedColor = "черное"
	}

	var (
		outcome      string
		balanceDelta float64
		displayWin   int
	)

	if choice == landedColor {
		outcome = "win"
		multiplier := 1
		if landedColor == "зеленое" {
			multiplier = 35
		}
		winAmount := req.Bet * multiplier
		balanceDelta = float64(winAmount)
		displayWin = winAmount + req.Bet

		if err := cache.ChangeBalance(req.UserID, balanceDelta); err != nil {
			log.Printf("Ошибка изменения баланса: %v", err)
			writeCasinoJSON(w, http.StatusInternalServerError, casinoResponse{
				OK: false, Code: "BALANCE_UPDATE_FAILED", Error: "failed to update balance",
			})
			return
		}
		if err := helpers.AddUserToWins(req.UserID); err != nil {
			log.Printf("Ошибка изменения рекордов: %v", err)
			writeCasinoJSON(w, http.StatusInternalServerError, casinoResponse{
				OK: false, Code: "RECORDS_UPDATE_FAILED", Error: "failed to update win records",
			})
			return
		}
	} else {
		outcome = "lose"
		balanceDelta = float64(-req.Bet)
		displayWin = -req.Bet

		if err := cache.ChangeBalance(req.UserID, balanceDelta); err != nil {
			log.Printf("Ошибка изменения баланса: %v", err)
			writeCasinoJSON(w, http.StatusInternalServerError, casinoResponse{
				OK: false, Code: "BALANCE_UPDATE_FAILED", Error: "failed to update balance",
			})
			return
		}
		if err := helpers.AddUserToLosses(req.UserID); err != nil {
			log.Printf("Ошибка изменения рекордов: %v", err)
			writeCasinoJSON(w, http.StatusInternalServerError, casinoResponse{
				OK: false, Code: "RECORDS_UPDATE_FAILED", Error: "failed to update loss records",
			})
			return
		}
	}

	newBalance, err := cache.GetBalance(req.UserID, req.Username)
	if err != nil {
		newBalance = 0
	}

	writeCasinoJSON(w, http.StatusOK, casinoResponse{
		OK: true,
		Data: map[string]any{
			"spin":          spin,
			"landed_color":  landedColor,
			"choice":        choice,
			"outcome":       outcome,
			"bet":           req.Bet,
			"balance_delta": balanceDelta,
			"display_win":   displayWin,
			"balance":       newBalance,
		},
	})
}