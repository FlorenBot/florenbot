package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	cache "florenbot/backend/engine/cache"
	helpers "florenbot/backend/engine/helpers"
	engine "florenbot/backend/engine/mysql"
	structs "florenbot/backend/engine/structs"
	std_helpers "florenbot/bot/helpers"
	"florenbot/services"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

const createPromoMinBalance = 2000.0

type PromoHandler struct {
	HTTP *services.HTTPService
}

func NewPromoHandler(httpService *services.HTTPService) *PromoHandler {
	return &PromoHandler{
		HTTP: httpService,
	}
}

func (h *PromoHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/promos", h.CreatePromo)
	mux.HandleFunc("GET /api/v1/promos", h.ListUserPromos)
	mux.HandleFunc("POST /api/v1/promos/activate", h.ActivatePromo)
	mux.HandleFunc("GET /api/v1/promos/stats", h.StatsPromos)
	mux.HandleFunc("PATCH /api/v1/promos/{code}", h.EditPromo)
	mux.HandleFunc("DELETE /api/v1/promos/{code}", h.DeletePromo)
	mux.HandleFunc("POST /api/v1/promos/{code}/expire", h.SetPromoExpire)
	mux.HandleFunc("DELETE /api/v1/promos/{code}/expire", h.DeletePromoExpire)
}

type promoRequest struct {
	UserID    uint64  `json:"user_id" example:"123456789"`
	Username  string  `json:"username,omitempty" example:"egorka"`
	FirstName string  `json:"first_name,omitempty" example:"Egor"`
	ChatID    int64   `json:"chat_id,omitempty" example:"-1001234567890"`
	Code      string  `json:"code,omitempty" example:"FLOREN2026"`
	AmountInt int     `json:"amount_int,omitempty" example:"500"`
	Amount    float64 `json:"amount,omitempty" example:"500"`
	Duration  string  `json:"duration,omitempty" example:"24h"`
}

type promoAPIResponse struct {
	OK    bool   `json:"ok" example:"true"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

type promoStatItem struct {
	Code      string `json:"code" example:"FLOREN2026"`
	Users     int64  `json:"users" example:"15"`
	CreatedAt string `json:"created_at" example:"24.11.2026"`
	UpdatedAt string `json:"updated_at" example:"-"`
}

func writePromoJSON(w http.ResponseWriter, status int, payload promoAPIResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func decodePromoRequest(r *http.Request) (promoRequest, error) {
	defer r.Body.Close()
	var req promoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, err
	}
	return req, nil
}

func ensurePromoUser(userID uint64, username, firstName string, debugType bool) (int, string, string, error) {
	if userID == 0 {
		return http.StatusBadRequest, "INVALID_USER", "user_id is required", errors.New("user_id is required")
	}

	userProfile, err := helpers.GetUserByID(userID)
	if err != nil {
		return http.StatusUnauthorized, "UNAUTHORIZED", "user is not authorized", err
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
			return http.StatusInternalServerError, "USER_CREATE_FAILED", "failed to create user", err
		}
		if debugType {
			log.Printf("Пользователь %s создан", firstName)
			log.Printf("Результат: %v", userProfile)
		}
	} else if debugType {
		log.Printf("Пользователь %s уже существует", firstName)
	}

	return http.StatusOK, "", "", nil
}

func (h *PromoHandler) checkCreatorViaService(ctx context.Context, chatID int64, userID int64) (bool, error) {
	err := godotenv.Load()
	if err != nil {
		log.Println("env not found, using system environment variables")
	}

	if h.HTTP == nil || chatID == 0 {
		return false, nil
	}
	baseURL := strings.TrimSpace(os.Getenv("BACKEND_API_URL"))
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("API_URL"))
	}
	if baseURL == "" {
		panic("NO API URL!")
	}
	q := url.Values{
		"chat_id": {strconv.FormatInt(chatID, 10)},
		"user_id": {strconv.FormatInt(userID, 10)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/v1/auth/is-creator?"+q.Encode(), nil)
	if err != nil {
		return false, err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var body struct {
		OK   bool `json:"ok"`
		Data struct {
			IsCreator bool `json:"is_creator"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, err
	}
	return body.Data.IsCreator, nil
}

// CreatePromo godoc
// @Summary Создание промокода
// @Description Создаёт новый промокод при достаточном балансе пользователя (минимум 2000)
// @Tags promos
// @Accept json
// @Produce json
// @Param request body promoRequest true "Данные для создания промокода"
// @Success 201 {object} promoAPIResponse
// @Failure 400 {object} promoAPIResponse
// @Failure 401 {object} promoAPIResponse
// @Failure 402 {object} promoAPIResponse
// @Failure 409 {object} promoAPIResponse
// @Failure 500 {object} promoAPIResponse
// @Router /api/v1/promos [post]
func (h *PromoHandler) CreatePromo(w http.ResponseWriter, r *http.Request) {
	req, err := decodePromoRequest(r)
	if err != nil {
		writePromoJSON(w, http.StatusBadRequest, promoAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}
	debugType := isDebugEnabled()

	if status, code, msg, err := ensurePromoUser(req.UserID, req.Username, req.FirstName, debugType); err != nil {
		writePromoJSON(w, status, promoAPIResponse{OK: false, Code: code, Error: msg})
		return
	}

	balance, err := cache.GetBalance(req.UserID, req.Username)
	if err != nil {
		if debugType {
			log.Printf("Ошибка получения баланса: %v", err)
		}
		writePromoJSON(w, http.StatusUnauthorized, promoAPIResponse{OK: false, Code: "UNAUTHORIZED", Error: "user is not authorized"})
		return
	}

	if balance < createPromoMinBalance {
		writePromoJSON(w, http.StatusPaymentRequired, promoAPIResponse{OK: false, Code: "INSUFFICIENT_FUNDS", Error: "insufficient funds, required 2000"})
		return
	}

	code := strings.TrimSpace(req.Code)
	if code == "" {
		writePromoJSON(w, http.StatusBadRequest, promoAPIResponse{OK: false, Code: "INVALID_CODE", Error: "promo code is required"})
		return
	}

	if _, err := helpers.GetCode(code); err == nil {
		writePromoJSON(w, http.StatusConflict, promoAPIResponse{OK: false, Code: "ALREADY_EXISTS", Error: "promocode already exists"})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		if debugType {
			log.Printf("Ошибка БД при проверке кода: %v", err)
		}
		writePromoJSON(w, http.StatusInternalServerError, promoAPIResponse{OK: false, Code: "DB_ERROR", Error: "database error"})
		return
	}

	if err := helpers.CreateCode(code, req.AmountInt, req.UserID); err != nil {
		if debugType {
			log.Printf("Ошибка при создании кода: %v", err)
		}
		writePromoJSON(w, http.StatusInternalServerError, promoAPIResponse{OK: false, Code: "CREATE_FAILED", Error: err.Error()})
		return
	}

	writePromoJSON(w, http.StatusCreated, promoAPIResponse{
		OK: true,
		Data: map[string]any{
			"code":   code,
			"amount": req.AmountInt,
		},
	})
}

// DeletePromo godoc
// @Summary Удаление промокода
// @Description Удаляет промокод по коду (доступно владельцу промокода или создателю чата)
// @Tags promos
// @Produce json
// @Param code path string true "Код промокода"
// @Param user_id query int true "ID пользователя"
// @Param chat_id query int false "ID чата"
// @Success 200 {object} promoAPIResponse
// @Failure 400 {object} promoAPIResponse
// @Failure 401 {object} promoAPIResponse
// @Failure 403 {object} promoAPIResponse
// @Failure 404 {object} promoAPIResponse
// @Failure 500 {object} promoAPIResponse
// @Router /api/v1/promos/{code} [delete]
func (h *PromoHandler) DeletePromo(w http.ResponseWriter, r *http.Request) {
	debugType := isDebugEnabled()
	code := strings.TrimSpace(r.PathValue("code"))
	if code == "" {
		writePromoJSON(w, http.StatusBadRequest, promoAPIResponse{OK: false, Code: "INVALID_CODE", Error: "promo code is required"})
		return
	}

	userID, err := parseUserIDParam(r)
	if err != nil {
		writePromoJSON(w, http.StatusBadRequest, promoAPIResponse{OK: false, Code: "INVALID_USER", Error: err.Error()})
		return
	}

	if _, err := helpers.GetUserByID(userID); err != nil {
		if debugType {
			log.Printf("Ошибка получения пользователя: %v", err)
		}
		writePromoJSON(w, http.StatusUnauthorized, promoAPIResponse{OK: false, Code: "UNAUTHORIZED", Error: "user is not authorized"})
		return
	}

	promo, err := helpers.GetCode(code)
	if err != nil {
		if debugType {
			log.Printf("Ошибка БД при поиске кода: %v", err)
		}
		if err.Error() == "sql: no rows in result set" || errors.Is(err, gorm.ErrRecordNotFound) {
			writePromoJSON(w, http.StatusNotFound, promoAPIResponse{OK: false, Code: "NOT_FOUND", Error: "promocode not found"})
			return
		}
		writePromoJSON(w, http.StatusInternalServerError, promoAPIResponse{OK: false, Code: "DB_ERROR", Error: "database error"})
		return
	}

	var chatID int64
	if rawChat := strings.TrimSpace(r.URL.Query().Get("chat_id")); rawChat != "" {
		chatID, _ = strconv.ParseInt(rawChat, 10, 64)
	}

	isCreator, err := h.checkCreatorViaService(r.Context(), chatID, int64(userID))
	if err != nil && debugType {
		log.Printf("Ошибка при проверке создателя: %v", err)
	}

	if isCreator {
		if debugType {
			log.Printf("Удаление промокода %s", code)
		}
		if err := helpers.DeleteCode(code); err != nil {
			if debugType {
				log.Printf("Ошибка при удалении: %v", err)
			}
			writePromoJSON(w, http.StatusInternalServerError, promoAPIResponse{OK: false, Code: "DELETE_FAILED", Error: "failed to delete promocode"})
			return
		}
		writePromoJSON(w, http.StatusOK, promoAPIResponse{
			OK: true,
			Data: map[string]any{
				"code":       code,
				"by_creator": true,
			},
		})
		return
	}

	if uint64(promo.OwnerID) != userID {
		writePromoJSON(w, http.StatusForbidden, promoAPIResponse{OK: false, Code: "FORBIDDEN", Error: "not allowed to delete this promocode"})
		return
	}

	if err := helpers.DeleteCode(code); err != nil {
		if debugType {
			log.Printf("Ошибка при удалении: %v", err)
		}
		writePromoJSON(w, http.StatusInternalServerError, promoAPIResponse{OK: false, Code: "DELETE_FAILED", Error: "failed to delete promocode"})
		return
	}

	writePromoJSON(w, http.StatusOK, promoAPIResponse{
		OK: true,
		Data: map[string]any{
			"code":       code,
			"by_creator": false,
		},
	})
}

// ActivatePromo godoc
// @Summary Активация промокода
// @Description Активирует промокод для пользователя и начисляет бонус на баланс
// @Tags promos
// @Accept json
// @Produce json
// @Param request body promoRequest true "Данные для активации промокода"
// @Success 200 {object} promoAPIResponse
// @Failure 400 {object} promoAPIResponse
// @Failure 404 {object} promoAPIResponse
// @Failure 409 {object} promoAPIResponse
// @Failure 500 {object} promoAPIResponse
// @Router /api/v1/promos/activate [post]
func (h *PromoHandler) ActivatePromo(w http.ResponseWriter, r *http.Request) {
	req, err := decodePromoRequest(r)
	if err != nil {
		writePromoJSON(w, http.StatusBadRequest, promoAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}
	debugType := isDebugEnabled()

	code := strings.TrimSpace(req.Code)
	if code == "" {
		writePromoJSON(w, http.StatusBadRequest, promoAPIResponse{OK: false, Code: "INVALID_CODE", Error: "promo code is required"})
		return
	}

	if status, errCode, msg, err := ensurePromoUser(req.UserID, req.Username, req.FirstName, debugType); err != nil {
		writePromoJSON(w, status, promoAPIResponse{OK: false, Code: errCode, Error: msg})
		return
	}

	existingPromo, err := helpers.GetUserCode(req.UserID)
	if err != nil || existingPromo != "" {
		writePromoJSON(w, http.StatusConflict, promoAPIResponse{OK: false, Code: "ALREADY_ACTIVATED", Error: "user already has an activated promocode"})
		return
	}

	promo, err := helpers.GetCode(code)
	if err != nil {
		if debugType {
			log.Printf("Ошибка БД при поиске кода: %v", err)
		}
		writePromoJSON(w, http.StatusNotFound, promoAPIResponse{OK: false, Code: "NOT_FOUND", Error: "promocode not found"})
		return
	}

	if uint64(promo.OwnerID) == req.UserID {
		writePromoJSON(w, http.StatusBadRequest, promoAPIResponse{OK: false, Code: "SELF_PROMO", Error: "cannot activate own promocode"})
		return
	}

	amount := promo.Amount

	if err := helpers.ActivateCode(req.UserID, code); err != nil {
		if debugType {
			log.Printf("Ошибка при активации: %v", err)
		}
		writePromoJSON(w, http.StatusInternalServerError, promoAPIResponse{OK: false, Code: "ACTIVATE_FAILED", Error: "failed to activate promocode"})
		return
	}

	currentBalance, err := cache.GetBalance(req.UserID, req.Username)
	if err != nil && debugType {
		log.Printf("Ошибка получения баланса: %v", err)
	}

	newBalance := currentBalance + amount

	if err := engine.UpdateBalance(req.UserID, amount); err != nil {
		if debugType {
			log.Printf("Ошибка обновления БД: %v", err)
		}
		writePromoJSON(w, http.StatusInternalServerError, promoAPIResponse{OK: false, Code: "BALANCE_UPDATE_FAILED", Error: "failed to update balance"})
		return
	}

	key := fmt.Sprintf("user_%d_balance", req.UserID)
	cache.SetCache(key, strconv.FormatFloat(newBalance, 'f', 2, 64), 24*time.Hour)

	writePromoJSON(w, http.StatusOK, promoAPIResponse{
		OK: true,
		Data: map[string]any{
			"code":        code,
			"amount":      amount,
			"new_balance": newBalance,
		},
	})
}

// ListUserPromos godoc
// @Summary Список промокодов пользователя
// @Description Возвращает промокоды, созданные указанным пользователем
// @Tags promos
// @Produce json
// @Param user_id query int true "ID пользователя"
// @Success 200 {object} promoAPIResponse
// @Failure 400 {object} promoAPIResponse
// @Failure 500 {object} promoAPIResponse
// @Router /api/v1/promos [get]
func (h *PromoHandler) ListUserPromos(w http.ResponseWriter, r *http.Request) {
	debugType := isDebugEnabled()
	userID, err := parseUserIDParam(r)
	if err != nil {
		writePromoJSON(w, http.StatusBadRequest, promoAPIResponse{OK: false, Code: "INVALID_USER", Error: err.Error()})
		return
	}

	promocodes, err := helpers.GetPromocodesUser(userID)
	if err != nil {
		if debugType {
			log.Printf("Ошибка при получении промокодов: %v", err)
		}
		writePromoJSON(w, http.StatusInternalServerError, promoAPIResponse{OK: false, Code: "LIST_FAILED", Error: "failed to load promocodes"})
		return
	}

	writePromoJSON(w, http.StatusOK, promoAPIResponse{
		OK: true,
		Data: map[string]any{
			"promocodes": promocodes,
		},
	})
}

// EditPromo godoc
// @Summary Редактирование промокода
// @Description Обновляет параметры промокода его владельцем
// @Tags promos
// @Accept json
// @Produce json
// @Param code path string true "Код промокода"
// @Param request body promoRequest true "Новые параметры промокода"
// @Success 200 {object} promoAPIResponse
// @Failure 400 {object} promoAPIResponse
// @Failure 403 {object} promoAPIResponse
// @Failure 404 {object} promoAPIResponse
// @Failure 500 {object} promoAPIResponse
// @Router /api/v1/promos/{code} [patch]
func (h *PromoHandler) EditPromo(w http.ResponseWriter, r *http.Request) {
	req, err := decodePromoRequest(r)
	if err != nil {
		writePromoJSON(w, http.StatusBadRequest, promoAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}
	debugType := isDebugEnabled()

	code := strings.TrimSpace(r.PathValue("code"))
	if code == "" {
		code = strings.TrimSpace(req.Code)
	}
	if code == "" {
		writePromoJSON(w, http.StatusBadRequest, promoAPIResponse{OK: false, Code: "INVALID_CODE", Error: "promo code is required"})
		return
	}

	if status, errCode, msg, err := ensurePromoUser(req.UserID, req.Username, req.FirstName, debugType); err != nil {
		writePromoJSON(w, status, promoAPIResponse{OK: false, Code: errCode, Error: msg})
		return
	}

	promo, err := helpers.GetCode(code)
	if err != nil {
		writePromoJSON(w, http.StatusNotFound, promoAPIResponse{OK: false, Code: "NOT_FOUND", Error: "promocode not found"})
		return
	}

	if uint64(promo.OwnerID) != req.UserID {
		writePromoJSON(w, http.StatusForbidden, promoAPIResponse{OK: false, Code: "FORBIDDEN", Error: "not the owner of this promocode"})
		return
	}

	if err := helpers.UpdatePromo(code, code, req.Amount, req.UserID); err != nil {
		writePromoJSON(w, http.StatusInternalServerError, promoAPIResponse{OK: false, Code: "UPDATE_FAILED", Error: "failed to update promocode"})
		return
	}

	writePromoJSON(w, http.StatusOK, promoAPIResponse{
		OK: true,
		Data: map[string]any{
			"code":   code,
			"amount": req.Amount,
		},
	})
}

// SetPromoExpire godoc
// @Summary Установка срока действия промокода
// @Description Задаёт время истечения промокода и сохраняет таймер в Redis
// @Tags promos
// @Accept json
// @Produce json
// @Param code path string true "Код промокода"
// @Param request body promoRequest true "Длительность действия промокода"
// @Success 200 {object} promoAPIResponse
// @Failure 400 {object} promoAPIResponse
// @Failure 403 {object} promoAPIResponse
// @Failure 404 {object} promoAPIResponse
// @Failure 409 {object} promoAPIResponse
// @Failure 500 {object} promoAPIResponse
// @Router /api/v1/promos/{code}/expire [post]
func (h *PromoHandler) SetPromoExpire(w http.ResponseWriter, r *http.Request) {
	req, err := decodePromoRequest(r)
	if err != nil {
		writePromoJSON(w, http.StatusBadRequest, promoAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}
	debugType := isDebugEnabled()

	code := strings.TrimSpace(r.PathValue("code"))
	if code == "" {
		code = strings.TrimSpace(req.Code)
	}
	if code == "" {
		writePromoJSON(w, http.StatusBadRequest, promoAPIResponse{OK: false, Code: "INVALID_CODE", Error: "promo code is required"})
		return
	}

	if status, errCode, msg, err := ensurePromoUser(req.UserID, req.Username, req.FirstName, debugType); err != nil {
		writePromoJSON(w, status, promoAPIResponse{OK: false, Code: errCode, Error: msg})
		return
	}

	promo, err := helpers.GetCode(code)
	if err != nil {
		writePromoJSON(w, http.StatusNotFound, promoAPIResponse{OK: false, Code: "NOT_FOUND", Error: "invalid code"})
		return
	}

	if uint64(promo.OwnerID) != req.UserID {
		writePromoJSON(w, http.StatusForbidden, promoAPIResponse{OK: false, Code: "FORBIDDEN", Error: "not the owner of this code"})
		return
	}

	if !promo.ExpiresAt.IsZero() {
		writePromoJSON(w, http.StatusConflict, promoAPIResponse{OK: false, Code: "ALREADY_HAS_EXPIRE", Error: "promocode already has an expiration date"})
		return
	}

	duration, err := std_helpers.ParseDuration(strings.TrimSpace(req.Duration))
	if err != nil {
		writePromoJSON(w, http.StatusBadRequest, promoAPIResponse{OK: false, Code: "INVALID_DURATION", Error: "invalid duration format"})
		return
	}

	expireTime := time.Now().Add(duration)
	expiresAtStr := expireTime.Format("2006-01-02 15:04:05")

	if err := helpers.UpdatePromoExpire(code, expiresAtStr); err != nil {
		log.Printf("Ошибка обновления БД для кода %s: %v", code, err)
		writePromoJSON(w, http.StatusInternalServerError, promoAPIResponse{OK: false, Code: "DB_ERROR", Error: "failed to update expiration in database"})
		return
	}

	timerData := structs.PromoTimer{
		UserID:      int64(req.UserID),
		PromoString: code,
	}
	data, _ := json.Marshal(timerData)

	redisKey := "promo_expire:" + code
	if err := cache.GetRedis().Set(context.Background(), redisKey, data, duration).Err(); err != nil {
		log.Printf("Ошибка записи в Redis: %v", err)
		writePromoJSON(w, http.StatusInternalServerError, promoAPIResponse{OK: false, Code: "REDIS_ERROR", Error: "timer system error"})
		return
	}

	writePromoJSON(w, http.StatusOK, promoAPIResponse{
		OK: true,
		Data: map[string]any{
			"code":              code,
			"expires_at":        expiresAtStr,
			"formatted_expires": expireTime.Format("02.01.2006 15:04"),
		},
	})
}

// DeletePromoExpire godoc
// @Summary Удаление срока действия промокода
// @Description Снимает ограничение по времени с промокода и удаляет таймер из Redis
// @Tags promos
// @Produce json
// @Param code path string true "Код промокода"
// @Param user_id query int true "ID пользователя"
// @Success 200 {object} promoAPIResponse
// @Failure 400 {object} promoAPIResponse
// @Failure 401 {object} promoAPIResponse
// @Failure 403 {object} promoAPIResponse
// @Failure 404 {object} promoAPIResponse
// @Failure 409 {object} promoAPIResponse
// @Failure 500 {object} promoAPIResponse
// @Failure 503 {object} promoAPIResponse
// @Router /api/v1/promos/{code}/expire [delete]
func (h *PromoHandler) DeletePromoExpire(w http.ResponseWriter, r *http.Request) {
	debugType := isDebugEnabled()
	code := strings.TrimSpace(r.PathValue("code"))
	if code == "" {
		writePromoJSON(w, http.StatusBadRequest, promoAPIResponse{OK: false, Code: "INVALID_CODE", Error: "promo code is required"})
		return
	}

	userID, err := parseUserIDParam(r)
	if err != nil {
		writePromoJSON(w, http.StatusBadRequest, promoAPIResponse{OK: false, Code: "INVALID_USER", Error: err.Error()})
		return
	}

	if _, err := helpers.GetUserByID(userID); err != nil {
		writePromoJSON(w, http.StatusUnauthorized, promoAPIResponse{OK: false, Code: "UNAUTHORIZED", Error: "user is not authorized"})
		return
	}

	promo, err := helpers.GetCode(code)
	if err != nil {
		writePromoJSON(w, http.StatusNotFound, promoAPIResponse{OK: false, Code: "NOT_FOUND", Error: "promocode not found"})
		return
	}

	if uint64(promo.OwnerID) != userID {
		writePromoJSON(w, http.StatusForbidden, promoAPIResponse{OK: false, Code: "FORBIDDEN", Error: "not the owner of this code"})
		return
	}

	if promo.ExpiresAt.IsZero() {
		writePromoJSON(w, http.StatusConflict, promoAPIResponse{OK: false, Code: "NO_EXPIRE", Error: "promocode has no expiration date"})
		return
	}

	redisKey := fmt.Sprintf("promo_expire:%s:%d", code, userID)
	rdb := cache.GetRedis()
	if rdb == nil {
		writePromoJSON(w, http.StatusServiceUnavailable, promoAPIResponse{OK: false, Code: "REDIS_UNAVAILABLE", Error: "redis is unavailable"})
		return
	}

	deletedCount, err := rdb.Del(context.Background(), redisKey).Result()
	if err != nil {
		if debugType {
			log.Printf("Ошибка удаления из Redis для кода %s: %v", code, err)
		}
		writePromoJSON(w, http.StatusInternalServerError, promoAPIResponse{OK: false, Code: "REDIS_ERROR", Error: "failed to delete timer"})
		return
	}

	if err := helpers.DeletePromoExpire(code); err != nil {
		if debugType {
			log.Printf("Ошибка удаления срока для кода %s: %v", code, err)
		}
		writePromoJSON(w, http.StatusInternalServerError, promoAPIResponse{OK: false, Code: "DB_ERROR", Error: "failed to delete expiration in database"})
		return
	}

	writePromoJSON(w, http.StatusOK, promoAPIResponse{
		OK: true,
		Data: map[string]any{
			"code":          code,
			"timer_existed": deletedCount > 0,
		},
	})
}

// StatsPromos godoc
// @Summary Статистика по промокодам
// @Description Возвращает статистику активаций и дат создания/обновления промокодов
// @Tags promos
// @Produce json
// @Success 200 {object} promoAPIResponse{data=map[string][]promoStatItem}
// @Failure 500 {object} promoAPIResponse
// @Router /api/v1/promos/stats [get]
func (h *PromoHandler) StatsPromos(w http.ResponseWriter, r *http.Request) {
	debugType := isDebugEnabled()

	var promos []structs.UserPromo
	if err := engine.DB.Find(&promos).Error; err != nil {
		if debugType {
			log.Printf("Ошибка при получении промокодов: %v", err)
		}
		writePromoJSON(w, http.StatusInternalServerError, promoAPIResponse{OK: false, Code: "STATS_ERROR", Error: "failed to get promocode stats"})
		return
	}

	items := make([]promoStatItem, 0, len(promos))
	for _, promo := range promos {
		cleanCode := strings.TrimSpace(promo.Code)
		if cleanCode == "" {
			continue
		}

		var num int64
		engine.DB.Model(&structs.User{}).Where("promocode = ?", cleanCode).Count(&num)

		createdAtStr := "N/A"
		if !promo.CreatedAt.IsZero() {
			createdAtStr = promo.CreatedAt.Format("02.01.2006")
		}

		updatedAtStr := "-"
		if !promo.UpdatedAt.IsZero() && promo.UpdatedAt.After(promo.CreatedAt) {
			updatedAtStr = promo.UpdatedAt.Format("02.01.2006")
		}

		items = append(items, promoStatItem{
			Code:      cleanCode,
			Users:     num,
			CreatedAt: createdAtStr,
			UpdatedAt: updatedAtStr,
		})
	}

	writePromoJSON(w, http.StatusOK, promoAPIResponse{
		OK: true,
		Data: map[string]any{
			"stats": items,
		},
	})
}