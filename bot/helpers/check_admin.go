package helpers

import (
	"context"
	"fmt"
	"net/url"

	"florenbot/services"
	"florenbot/structs"
)

// IsUserAdmin проверяет права администратора для объекта роли
func IsUserAdmin(role *structs.Role) bool {
	if role == nil {
		return false
	}
	return role.BaseShort == "admin" || role.BaseShort == "owner" || role.BaseShort == "creator"
}

func IsUserOwner(role *structs.Role) bool {
	if role == nil {
		return false
	}
	return role.BaseShort == "owner"
}

func IsUserCreator(role *structs.Role) bool {
	if role == nil {
		return false
	}
	return role.BaseShort == "creator"
}

func IsUserOwnerOrCreator(role *structs.Role) bool {
	if role == nil {
		return false
	}
	return role.BaseShort == "owner" || role.BaseShort == "creator"
}

// MemberCheckResponse структура ответа от бэкенда
type MemberCheckResponse struct {
	Status    string `json:"status"`
	IsCreator bool   `json:"is_creator"`
}

// IsCreator запрашивает статус пользователя через ваш HTTPService (apiClient)
func IsCreator(ctx context.Context, apiClient *services.HTTPService, chatID int64, userID int64) (bool, error) {
	queryParams := url.Values{}
	queryParams.Set("chat_id", fmt.Sprintf("%d", chatID))
	queryParams.Set("user_id", fmt.Sprintf("%d", userID))

	var resp MemberCheckResponse

	err := apiClient.Get(ctx, "/api/check-creator", queryParams, &resp)
	if err != nil {
		return false, fmt.Errorf("ошибка проверки статуса через API: %w", err)
	}

	return resp.IsCreator || resp.Status == "creator", nil
}