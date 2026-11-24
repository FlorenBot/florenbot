package helpers

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"florenbot/services"
)

// VIPCheckResponse структура ответа от бэкенда для проверки VIP-статуса
type VIPCheckResponse struct {
	IsVIP bool `json:"is_vip"`
}

// IsUserVIP проверяет, является ли пользователь VIP, через HTTP API
func IsUserVIP(ctx context.Context, apiClient *services.HTTPService, userID uint64) (bool, error) {
	queryParams := url.Values{}
	queryParams.Set("user_id", strconv.FormatUint(userID, 10))

	var resp VIPCheckResponse
	endpoint := fmt.Sprintf("/api/users/%d/vip", userID) // Или любой ваш эндпоинт на бэкенде
	
	err := apiClient.Get(ctx, endpoint, queryParams, &resp)
	if err != nil {
		return false, fmt.Errorf("ошибка проверки VIP-статуса через API: %w", err)
	}

	return resp.IsVIP, nil
}

// AddVip добавляет VIP-статус пользователю через HTTP API
func AddVip(ctx context.Context, apiClient *services.HTTPService, userID uint64, level int) error {
	payload := map[string]interface{}{
		"user_id": userID,
		"level":   level,
	}
	
	var resp interface{}
	return apiClient.PostJSON(ctx, "/api/users/vip", payload, &resp)
}