package helpers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"time"

	"florenbot/services"
	"florenbot/structs"
)

// ==================== Методы работы через HTTP ====================

// GetRole получает роль пользователя через HTTP
func GetRole(ctx context.Context, apiClient *services.HTTPService, userID uint64) (string, error) {
	var resp struct {
		Role string `json:"role"`
	}
	endpoint := fmt.Sprintf("/api/users/%d/role", userID)
	err := apiClient.Get(ctx, endpoint, nil, &resp)
	if err != nil {
		return "", err
	}
	return resp.Role, nil
}

// IsUserBanned проверяет, забанен ли пользователь через HTTP
func IsUserBanned(ctx context.Context, apiClient *services.HTTPService, userID uint64) bool {
	var resp struct {
		Banned bool `json:"banned"`
	}
	endpoint := fmt.Sprintf("/api/users/%d/banned", userID)
	err := apiClient.Get(ctx, endpoint, nil, &resp)
	if err != nil {
		return false
	}
	return resp.Banned
}

// GetUserIDByUsername получает ID пользователя по юзернейму через HTTP
func GetUserIDByUsername(ctx context.Context, apiClient *services.HTTPService, username string) uint64 {
	queryParams := url.Values{}
	queryParams.Set("username", username)

	var resp structs.User
	err := apiClient.Get(ctx, "/api/users/by-username", queryParams, &resp)
	if err != nil {
		return 0
	}
	return resp.ID
}

// GetUser получает пользователя через HTTP
func GetUser(ctx context.Context, apiClient *services.HTTPService, userID uint64) (*structs.User, error) {
	var user structs.User
	endpoint := fmt.Sprintf("/api/users/%d", userID)
	err := apiClient.Get(ctx, endpoint, nil, &user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByIDDB получает пользователя через HTTP
func GetUserByIDDB(ctx context.Context, apiClient *services.HTTPService, userID uint64) (structs.User, error) {
	var user structs.User
	endpoint := fmt.Sprintf("/api/users/%d", userID)
	err := apiClient.Get(ctx, endpoint, nil, &user)
	return user, err
}

// saveUserToCache сохраняет пользователя через HTTP-эндпоинт кэша
func saveUserToCache(ctx context.Context, apiClient *services.HTTPService, user structs.User) {
	endpoint := fmt.Sprintf("/cache/user/%d", user.ID)
	var resp interface{}
	err := apiClient.PostJSON(ctx, endpoint, user, &resp)
	if err != nil {
		log.Printf("Ошибка сохранения пользователя %d в кэш через HTTP: %v", user.ID, err)
	}
}

// IncrementMessageCount увеличивает счетчик сообщений через HTTP
func IncrementMessageCount(ctx context.Context, apiClient *services.HTTPService, userID uint64, username string, firstName string) {
	_, err := GetOrCreateUser(ctx, apiClient, userID, username, firstName)
	if err != nil {
		log.Printf("Error getting or creating user: %v", err)
		return
	}

	log.Printf("Incrementing message count for user %d", userID)

	endpoint := fmt.Sprintf("/api/users/%d/increment-message", userID)
	var resp interface{}
	err = apiClient.PostJSON(ctx, endpoint, nil, &resp)
	if err != nil {
		log.Printf("Error incrementing message count: %v", err)
	}
}

// GetTopByBalance возвращает топ пользователей по балансу через HTTP
func GetTopByBalance(ctx context.Context, apiClient *services.HTTPService, limit int) ([]structs.User, error) {
	queryParams := url.Values{}
	queryParams.Set("limit", strconv.Itoa(limit))

	var topUsers []structs.User
	err := apiClient.Get(ctx, "/api/users/top/balance", queryParams, &topUsers)
	return topUsers, err
}

// GetTopByMessages возвращает топ пользователей по сообщениям через HTTP
func GetTopByMessages(ctx context.Context, apiClient *services.HTTPService, limit int) ([]structs.User, error) {
	queryParams := url.Values{}
	queryParams.Set("limit", strconv.Itoa(limit))

	var topUsers []structs.User
	err := apiClient.Get(ctx, "/api/users/top/messages", queryParams, &topUsers)
	return topUsers, err
}

// GetUserBalance получает текущий баланс пользователя через HTTP
func GetUserBalance(ctx context.Context, apiClient *services.HTTPService, userID uint64) (float64, error) {
	var resp struct {
		Balance float64 `json:"balance"`
	}
	
	endpoint := fmt.Sprintf("/api/users/%d/balance", userID)
	err := apiClient.Get(ctx, endpoint, nil, &resp)
	if err != nil {
		return 0, fmt.Errorf("ошибка получения баланса через API: %w", err)
	}

	return resp.Balance, nil
}

// GetUserByID пытается получить пользователя из кэша (/cache/get) или запрашивает бэкенд
func GetUserByID(ctx context.Context, apiClient *services.HTTPService, tgID uint64) (structs.User, error) {
	var user structs.User

	queryParams := url.Values{}
	queryParams.Set("key", fmt.Sprintf("user:%d", tgID))

	// 1. Попытка чтения из кэша через эндпоинт /cache/get
	err := apiClient.Get(ctx, "/cache/get", queryParams, &user)
	if err == nil && user.ID != 0 {
		return user, nil
	}

	// 2. Если в кэше нет — запрашиваем у основного API бэкенда
	endpoint := fmt.Sprintf("/api/users/%d", tgID)
	err = apiClient.Get(ctx, endpoint, nil, &user)
	if err != nil {
		return user, fmt.Errorf("пользователь %d не найден: %w", tgID, err)
	}

	// 3. Сохраняем в кэш после успешного получения из БД
	saveUserToCache(ctx, apiClient, user)

	return user, nil
}

// GetOrCreateUser находит или создает пользователя через HTTP
func GetOrCreateUser(ctx context.Context, apiClient *services.HTTPService, tgID uint64, username string, firstName string) (structs.User, error) {
	payload := map[string]interface{}{
		"id":         tgID,
		"username":   username,
		"first_name": firstName,
	}

	var user structs.User
	err := apiClient.PostJSON(ctx, "/api/users/get-or-create", payload, &user)
	if err != nil {
		return structs.User{}, err
	}

	saveUserToCache(ctx, apiClient, user)
	return user, nil
}

// IsCommandRestricted проверяет ограничения команд через HTTP
func IsCommandRestricted(ctx context.Context, apiClient *services.HTTPService, userID uint64, chatID uint64, command string) (bool, error) {
	queryParams := url.Values{}
	queryParams.Set("user_id", strconv.FormatUint(userID, 10))
	queryParams.Set("chat_id", strconv.FormatUint(chatID, 10))
	queryParams.Set("command", command)

	var resp struct {
		Restricted bool `json:"restricted"`
	}
	err := apiClient.Get(ctx, "/api/commands/restricted", queryParams, &resp)
	if err != nil {
		return false, err
	}

	return resp.Restricted, nil
}

// GetLastBonusTime получает время последнего бонуса через HTTP
func GetLastBonusTime(ctx context.Context, apiClient *services.HTTPService, userID uint64) (time.Time, error) {
	var resp struct {
		LastBonusAt time.Time `json:"last_bonus_at"`
	}
	endpoint := fmt.Sprintf("/api/users/%d/bonus-time", userID)
	err := apiClient.Get(ctx, endpoint, nil, &resp)
	if err != nil {
		return time.Time{}, err
	}

	return resp.LastBonusAt, nil
}

// AddVip обновляет VIP уровень через HTTP
func AddVip(ctx context.Context, apiClient *services.HTTPService, userId uint64, vipLevel uint16) error {
	payload := map[string]interface{}{
		"vip_level": vipLevel,
	}
	endpoint := fmt.Sprintf("/api/users/%d/vip", userId)

	var resp interface{}
	err := apiClient.PostJSON(ctx, endpoint, payload, &resp)
	if err != nil {
		log.Printf("Ошибка при обновлении VIP уровня для пользователя %d: %v", userId, err)
		return errors.New("Ошибка при обновлении VIP уровня для пользователя")
	}

	return nil
}

// UpdateLastBonusTime обновляет время бонуса через HTTP
func UpdateLastBonusTime(ctx context.Context, apiClient *services.HTTPService, userID uint64, t time.Time) error {
	payload := map[string]interface{}{
		"last_bonus_at": t,
	}
	endpoint := fmt.Sprintf("/api/users/%d/bonus-time", userID)

	var resp interface{}
	err := apiClient.PostJSON(ctx, endpoint, payload, &resp)
	if err != nil {
		log.Printf("Ошибка при обновлении времени последнего бонуса для пользователя %d: %v", userID, err)
		return err
	}

	return nil
}

// UpdateUserCache обновляет кэш пользователя через HTTP
func UpdateUserCache(ctx context.Context, apiClient *services.HTTPService, user structs.User) {
	endpoint := fmt.Sprintf("/cache/update/%d", user.ID)
	var resp interface{}
	err := apiClient.PostJSON(ctx, endpoint, user, &resp)
	if err != nil {
		log.Printf("Ошибка обновления кэша пользователя %d через HTTP: %v", user.ID, err)
	}
}

// AddUserToLosses инкрементирует поражения через HTTP
func AddUserToLosses(ctx context.Context, apiClient *services.HTTPService, userID uint64) error {
	endpoint := fmt.Sprintf("/api/users/%d/losses", userID)
	var resp interface{}
	return apiClient.PostJSON(ctx, endpoint, nil, &resp)
}

// AddUserToWins инкрементирует победы через HTTP
func AddUserToWins(ctx context.Context, apiClient *services.HTTPService, userID uint64) error {
	endpoint := fmt.Sprintf("/api/users/%d/wins", userID)
	var resp interface{}
	return apiClient.PostJSON(ctx, endpoint, nil, &resp)
}

// CreateUser создает пользователя через HTTP
func CreateUser(ctx context.Context, apiClient *services.HTTPService, tgID uint64, username, firstName string) (structs.User, error) {
	payload := map[string]interface{}{
		"id":         tgID,
		"username":   username,
		"first_name": firstName,
	}
	var user structs.User
	err := apiClient.PostJSON(ctx, "/api/users", payload, &user)
	if err != nil {
		return structs.User{}, err
	}
	return user, nil
}

// GetUsers возвращает список всех пользователей через HTTP
func GetUsers(ctx context.Context, apiClient *services.HTTPService) ([]structs.User, error) {
	var users []structs.User
	err := apiClient.Get(ctx, "/api/users", nil, &users)
	return users, err
}