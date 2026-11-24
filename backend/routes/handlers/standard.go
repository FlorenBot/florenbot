package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"florenbot/backend/checks"
	"florenbot/backend/engine/cache"
	"florenbot/backend/engine/helpers"
	"florenbot/backend/engine/mysql"
	backend_structs "florenbot/backend/engine/structs"
	"florenbot/services"
	"gorm.io/gorm"
)

type StandardHandler struct {
	HTTP *services.HTTPService
}

func NewStandardHandler(httpService *services.HTTPService) *StandardHandler {
	return &StandardHandler{
		HTTP: httpService,
	}
}

func (h *StandardHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/standard/help-context", h.HelpContext)
	mux.HandleFunc("POST /api/v1/standard/profile/init", h.ProfileInit)
	mux.HandleFunc("POST /api/v1/standard/start/group", h.StartGroup)
	mux.HandleFunc("POST /api/v1/standard/bonus", h.ClaimBonus)
	mux.HandleFunc("POST /api/v1/standard/vip", h.ToggleOrClaimVIP)
	mux.HandleFunc("GET /api/v1/standard/top/balance", h.TopBalance)
	mux.HandleFunc("GET /api/v1/standard/top/messages", h.TopMessages)
	mux.HandleFunc("GET /api/v1/standard/balance", h.GetBalance)
	mux.HandleFunc("GET /api/v1/standard/profile", h.GetProfile)
}

type standardDataPayload struct {
	RoleName         string               `json:"role_name,omitempty" example:"Гость"`
	RoleShort        string               `json:"role_short,omitempty" example:"moderator"`
	IsAdmin          bool                 `json:"is_admin,omitempty" example:"false"`
	IsOwnerOrCreator bool                 `json:"is_owner_or_creator,omitempty" example:"false"`
	IsCreator        bool                 `json:"is_creator,omitempty" example:"false"`
	Profile          backend_structs.User `json:"profile,omitempty"`
	ChatTitle        string               `json:"chat_title,omitempty"`
	RoleDisplay      string               `json:"role_display,omitempty"`
	IsCreatorInChat  bool                 `json:"is_creator_in_chat,omitempty"`
	RemainingHours   float64              `json:"remaining_hours,omitempty" example:"12"`
	BonusAmount      int                  `json:"bonus_amount,omitempty" example:"100"`
	NewBalance       float64              `json:"new_balance,omitempty" example:"500"`
	IsVIP            bool                 `json:"is_vip,omitempty" example:"true"`
	Users            []backend_structs.User `json:"users,omitempty"`
}

type standardAPIResponse struct {
	OK    bool                 `json:"ok" example:"true"`
	Code  string               `json:"code,omitempty"`
	Data  *standardDataPayload `json:"data,omitempty"`
	Error string               `json:"error,omitempty"`
}

type profileInitRequest struct {
	UserID    uint64 `json:"user_id" example:"123456789"`
	Username  string `json:"username,omitempty" example:"egorka"`
	FirstName string `json:"first_name,omitempty" example:"Egor"`
}

type startGroupRequest struct {
	UserID    uint64 `json:"user_id" example:"123456789"`
	Username  string `json:"username,omitempty" example:"egorka"`
	FirstName string `json:"first_name,omitempty" example:"Egor"`
	ChatID    int64  `json:"chat_id" example:"-1001234567890"`
	ChatTitle string `json:"chat_title,omitempty" example:"Test Chat"`
}

type bonusRequest struct {
	UserID    uint64 `json:"user_id" example:"123456789"`
	Username  string `json:"username,omitempty" example:"egorka"`
	FirstName string `json:"first_name,omitempty" example:"Egor"`
}

type vipRequest struct {
	UserID uint64 `json:"user_id" example:"123456789"`
}

func isStdHandlerDebug() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG")))
	return v == "1" || v == "true" || v == "yes"
}

func isRecordNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "not found") || strings.Contains(low, "не найден")
}

func writeStandardJSON(w http.ResponseWriter, status int, payload standardAPIResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func getBotSignatureSecret() string {
	for _, key := range []string{"APP_KEY", "INTERNAL_API_SECRET", "API_SIGNATURE_SECRET"} {
		if val := strings.TrimSpace(os.Getenv(key)); val != "" {
			return val
		}
	}
	return ""
}

func signStandardRequest(req *http.Request, body []byte) {
	secret := getBotSignatureSecret()
	if secret == "" {
		return
	}

	ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write([]byte(strings.ToUpper(req.Method)))
	mac.Write([]byte("."))
	mac.Write([]byte(req.URL.Path))
	mac.Write([]byte("."))
	mac.Write(body)

	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Signature", hex.EncodeToString(mac.Sum(nil)))
}

// HelpContext godoc
// @Summary Контекст ролей для /help
// @Description Возвращает роль пользователя и флаги прав для формирования меню команд
// @Tags standard
// @Produce json
// @Param user_id query int true "ID пользователя"
// @Param chat_id query int true "ID чата"
// @Success 200 {object} standardAPIResponse
// @Failure 400 {object} standardAPIResponse
// @Router /api/v1/standard/help-context [get]
func (h *StandardHandler) HelpContext(w http.ResponseWriter, r *http.Request) {
	dbg := isStdHandlerDebug()
	rawUserID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	rawChatID := strings.TrimSpace(r.URL.Query().Get("chat_id"))

	if dbg {
		log.Printf("[DEBUG][HELP_CONTEXT] user_id=%q, chat_id=%q", rawUserID, rawChatID)
	}

	writeStandardJSON(w, http.StatusOK, standardAPIResponse{
		OK: true,
		Data: &standardDataPayload{
			RoleName: "Гость",
		},
	})
}

// ProfileInit godoc
// @Summary Инициализация профиля в ЛС
// @Description Создаёт или возвращает профиль пользователя при старте в личных сообщениях
// @Tags standard
// @Accept json
// @Produce json
// @Param request body profileInitRequest true "Данные пользователя"
// @Success 200 {object} standardAPIResponse
// @Failure 400 {object} standardAPIResponse
// @Failure 401 {object} standardAPIResponse
// @Failure 500 {object} standardAPIResponse
// @Router /api/v1/standard/profile/init [post]
func (h *StandardHandler) ProfileInit(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	dbg := isStdHandlerDebug()

	if err := checks.VerifyRequest(r); err != nil {
		if dbg {
			log.Printf("[DEBUG][PROFILE_INIT] Ошибка подписи: %v", err)
		}
		writeStandardJSON(w, http.StatusUnauthorized, standardAPIResponse{
			OK:    false,
			Code:  "INVALID_SIGNATURE",
			Error: err.Error(),
		})
		return
	}

	var req profileInitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == 0 {
		writeStandardJSON(w, http.StatusBadRequest, standardAPIResponse{
			OK:    false,
			Error: "invalid request body",
		})
		return
	}

	username := strings.TrimSpace(req.Username)
	firstName := strings.TrimSpace(req.FirstName)

	if dbg {
		log.Printf("[DEBUG][PROFILE_INIT] Запрос для user_id=%d, username=%q, first_name=%q", req.UserID, username, firstName)
	}

	userProfile, err := helpers.GetUserByID(req.UserID)
	if err != nil {
		if !isRecordNotFoundErr(err) {
			if dbg {
				log.Printf("[DEBUG][PROFILE_INIT] Ошибка чтения из БД user_id=%d: %v", req.UserID, err)
			}
			writeStandardJSON(w, http.StatusInternalServerError, standardAPIResponse{
				OK:    false,
				Code:  "DB_ERROR",
				Error: "failed to load user profile",
			})
			return
		}
		if dbg {
			log.Printf("[DEBUG][PROFILE_INIT] Пользователь %d не найден в БД (%v), создаём...", req.UserID, err)
		}
		userProfile = backend_structs.User{}
	}

	if userProfile.ID == 0 {
		createdUser, createErr := helpers.CreateUser(req.UserID, username, firstName)
		if createErr != nil {
			if dbg {
				log.Printf("[DEBUG][PROFILE_INIT] Ошибка создания user_id=%d: %v", req.UserID, createErr)
			}
			writeStandardJSON(w, http.StatusInternalServerError, standardAPIResponse{
				OK:    false,
				Code:  "USER_CREATE_FAILED",
				Error: "failed to create user in database",
			})
			return
		}
		userProfile = createdUser
		if dbg {
			log.Printf("[DEBUG][PROFILE_INIT] Создан пользователь: %+v", userProfile)
		}
	} else if dbg {
		log.Printf("[DEBUG][PROFILE_INIT] Найден пользователь в БД: %+v", userProfile)
	}

	// Синхронизируем баланс в кэше актуальным значением из БД
	cache.SetCache("user_"+strconv.FormatUint(req.UserID, 10)+"_balance", strconv.FormatFloat(userProfile.Balance, 'f', 2, 64), 24*time.Hour)

	writeStandardJSON(w, http.StatusOK, standardAPIResponse{
		OK: true,
		Data: &standardDataPayload{
			Profile: userProfile,
		},
	})
}

// StartGroup godoc
// @Summary Регистрация группового чата
// @Description Инициализирует чат и роли при вызове /start в группе или супергруппе
// @Tags standard
// @Accept json
// @Produce json
// @Param request body startGroupRequest true "Данные пользователя и чата"
// @Success 200 {object} standardAPIResponse
// @Failure 400 {object} standardAPIResponse
// @Router /api/v1/standard/start/group [post]
func (h *StandardHandler) StartGroup(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req startGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == 0 || req.ChatID == 0 {
		writeStandardJSON(w, http.StatusBadRequest, standardAPIResponse{
			OK:    false,
			Error: "invalid request body",
		})
		return
	}

	writeStandardJSON(w, http.StatusOK, standardAPIResponse{OK: true})
}

// ClaimBonus godoc
// @Summary Ежедневный бонус
// @Description Начисляет ежедневный бонус пользователю или возвращает оставшееся время ожидания
// @Tags standard
// @Accept json
// @Produce json
// @Param request body bonusRequest true "Данные пользователя"
// @Success 200 {object} standardAPIResponse
// @Failure 400 {object} standardAPIResponse
// @Router /api/v1/standard/bonus [post]
func (h *StandardHandler) ClaimBonus(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req bonusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == 0 {
		writeStandardJSON(w, http.StatusBadRequest, standardAPIResponse{
			OK:    false,
			Error: "invalid request body",
		})
		return
	}

	writeStandardJSON(w, http.StatusOK, standardAPIResponse{OK: true})
}

// ToggleOrClaimVIP godoc
// @Summary Выдача или проверка VIP-статуса
// @Description Активирует VIP-статус для пользователя или сообщает, что он уже активен
// @Tags standard
// @Accept json
// @Produce json
// @Param request body vipRequest true "ID пользователя"
// @Success 200 {object} standardAPIResponse
// @Failure 400 {object} standardAPIResponse
// @Router /api/v1/standard/vip [post]
func (h *StandardHandler) ToggleOrClaimVIP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req vipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == 0 {
		writeStandardJSON(w, http.StatusBadRequest, standardAPIResponse{
			OK:    false,
			Error: "invalid request body",
		})
		return
	}

	writeStandardJSON(w, http.StatusOK, standardAPIResponse{OK: true})
}

// TopBalance godoc
// @Summary Топ по балансу
// @Description Возвращает список пользователей с наибольшим балансом
// @Tags standard
// @Produce json
// @Param limit query int false "Лимит записей" default(10)
// @Success 200 {object} standardAPIResponse
// @Router /api/v1/standard/top/balance [get]
func (h *StandardHandler) TopBalance(w http.ResponseWriter, r *http.Request) {
	dbg := isStdHandlerDebug()
	limit := 10
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var users []backend_structs.User
	if mysql.DB != nil {
		if err := mysql.DB.Order("balance DESC").Limit(limit).Find(&users).Error; err != nil && dbg {
			log.Printf("[DEBUG][TOP_BALANCE] Ошибка запроса топа баланса: %v", err)
		}
	}

	writeStandardJSON(w, http.StatusOK, standardAPIResponse{
		OK: true,
		Data: &standardDataPayload{
			Users: users,
		},
	})
}

// TopMessages godoc
// @Summary Топ по сообщениям
// @Description Возвращает список пользователей по количеству сообщений
// @Tags standard
// @Produce json
// @Param limit query int false "Лимит записей" default(10)
// @Success 200 {object} standardAPIResponse
// @Router /api/v1/standard/top/messages [get]
func (h *StandardHandler) TopMessages(w http.ResponseWriter, r *http.Request) {
	dbg := isStdHandlerDebug()
	limit := 10
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var users []backend_structs.User
	if mysql.DB != nil {
		if err := mysql.DB.Order("message_count DESC").Limit(limit).Find(&users).Error; err != nil && dbg {
			log.Printf("[DEBUG][TOP_MESSAGES] Ошибка запроса топа сообщений: %v", err)
		}
	}

	writeStandardJSON(w, http.StatusOK, standardAPIResponse{
		OK: true,
		Data: &standardDataPayload{
			Users: users,
		},
	})
}

// GetBalance godoc
// @Summary Баланс пользователя
// @Description Возвращает текущий баланс пользователя напрямую из БД
// @Tags standard
// @Produce json
// @Param user_id query int true "ID пользователя"
// @Success 200 {object} standardAPIResponse
// @Failure 400 {object} standardAPIResponse
// @Failure 404 {object} standardAPIResponse
// @Failure 500 {object} standardAPIResponse
// @Router /api/v1/standard/balance [get]
func (h *StandardHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	dbg := isStdHandlerDebug()
	rawID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	if rawID == "" {
		writeStandardJSON(w, http.StatusBadRequest, standardAPIResponse{
			OK:    false,
			Error: "user_id is required",
		})
		return
	}

	userID, err := strconv.ParseUint(rawID, 10, 64)
	if err != nil || userID == 0 {
		writeStandardJSON(w, http.StatusBadRequest, standardAPIResponse{
			OK:    false,
			Error: "invalid user_id",
		})
		return
	}

	if dbg {
		log.Printf("[DEBUG][GET_BALANCE] Чтение баланса из БД для user_id=%d", userID)
	}

	userProfile, err := helpers.GetUserByID(userID)
	if err != nil {
		if isRecordNotFoundErr(err) {
			if dbg {
				log.Printf("[DEBUG][GET_BALANCE] Пользователь %d не найден: %v", userID, err)
			}
			writeStandardJSON(w, http.StatusNotFound, standardAPIResponse{
				OK:    false,
				Code:  "USER_NOT_FOUND",
				Error: "user not found",
			})
			return
		}
		if dbg {
			log.Printf("[DEBUG][GET_BALANCE] Ошибка БД при получении баланса user_id=%d: %v", userID, err)
		}
		writeStandardJSON(w, http.StatusInternalServerError, standardAPIResponse{
			OK:    false,
			Code:  "DB_ERROR",
			Error: "failed to load user balance from database",
		})
		return
	}

	if dbg {
		log.Printf("[DEBUG][GET_BALANCE] Баланс из БД для user_id=%d: %.2f", userID, userProfile.Balance)
	}

	cache.SetCache("user_"+strconv.FormatUint(userID, 10)+"_balance", strconv.FormatFloat(userProfile.Balance, 'f', 2, 64), 24*time.Hour)

	writeStandardJSON(w, http.StatusOK, standardAPIResponse{
		OK: true,
		Data: &standardDataPayload{
			NewBalance: userProfile.Balance,
			Profile:    userProfile,
		},
	})
}

// GetProfile godoc
// @Summary Профиль пользователя
// @Description Возвращает подробный профиль пользователя и данные чата из БД
// @Tags standard
// @Produce json
// @Param user_id query int true "ID пользователя"
// @Param chat_id query int false "ID чата"
// @Param chat_type query string false "Тип чата"
// @Param username query string false "Username пользователя"
// @Param first_name query string false "Имя пользователя"
// @Success 200 {object} standardAPIResponse
// @Failure 400 {object} standardAPIResponse
// @Failure 404 {object} standardAPIResponse
// @Failure 500 {object} standardAPIResponse
// @Router /api/v1/standard/profile [get]
func (h *StandardHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	dbg := isStdHandlerDebug()
	rawID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	if rawID == "" {
		writeStandardJSON(w, http.StatusBadRequest, standardAPIResponse{
			OK:    false,
			Error: "user_id is required",
		})
		return
	}

	userID, err := strconv.ParseUint(rawID, 10, 64)
	if err != nil || userID == 0 {
		writeStandardJSON(w, http.StatusBadRequest, standardAPIResponse{
			OK:    false,
			Error: "invalid user_id",
		})
		return
	}

	if dbg {
		log.Printf("[DEBUG][GET_PROFILE] Чтение профиля из БД для user_id=%d", userID)
	}

	userProfile, err := helpers.GetUserByID(userID)
	if err != nil {
		if isRecordNotFoundErr(err) {
			if dbg {
				log.Printf("[DEBUG][GET_PROFILE] Пользователь %d не найден: %v", userID, err)
			}
			writeStandardJSON(w, http.StatusNotFound, standardAPIResponse{
				OK:    false,
				Code:  "USER_NOT_FOUND",
				Error: "user not found",
			})
			return
		}
		if dbg {
			log.Printf("[DEBUG][GET_PROFILE] Ошибка БД при получении профиля user_id=%d: %v", userID, err)
		}
		writeStandardJSON(w, http.StatusInternalServerError, standardAPIResponse{
			OK:    false,
			Code:  "DB_ERROR",
			Error: "failed to load user profile from database",
		})
		return
	}

	if dbg {
		log.Printf("[DEBUG][GET_PROFILE] Профиль загружен из БД: %+v", userProfile)
	}

	writeStandardJSON(w, http.StatusOK, standardAPIResponse{
		OK: true,
		Data: &standardDataPayload{
			Profile: userProfile,
		},
	})
}