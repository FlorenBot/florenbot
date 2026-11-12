package helpers

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"florenbot/services"
	"florenbot/structs"
)

// GetRolesByChatID возвращает список всех ролей для чата через HTTP
func GetRolesByChatID(ctx context.Context, apiClient *services.HTTPService, chat_id uint64) ([]structs.Role, error) {
	var roles []structs.Role
	endpoint := fmt.Sprintf("/api/chats/%d/roles", chat_id)
	err := apiClient.Get(ctx, endpoint, nil, &roles)
	return roles, err
}

// EditRole обновляет роль и возвращает обновленный список ролей через HTTP
func EditRole(ctx context.Context, apiClient *services.HTTPService, role structs.Role) ([]structs.Role, error) {
	endpoint := fmt.Sprintf("/api/roles/%d", role.ID)
	
	payload := map[string]interface{}{
		"name":       role.Name,
		"short_name": role.ShortName,
		"chat_id":    role.ChatID,
	}

	var updatedRoles []structs.Role
	err := apiClient.PutJSON(ctx, endpoint, payload, &updatedRoles)
	if err != nil {
		return nil, err
	}

	return updatedRoles, nil
}

// GetUsersByRole получает пользователей по роли через HTTP
func GetUsersByRole(ctx context.Context, apiClient *services.HTTPService, roleID uint64, chatID uint64) ([]structs.User, error) {
	queryParams := url.Values{}
	queryParams.Set("chat_id", strconv.FormatUint(chatID, 10))

	var users []structs.User
	endpoint := fmt.Sprintf("/api/roles/%d/users", roleID)
	err := apiClient.Get(ctx, endpoint, queryParams, &users)
	return users, err
}

// SetRole устанавливает роль пользователю через HTTP
func SetRole(ctx context.Context, apiClient *services.HTTPService, user_id uint64, role_id uint64, chat_id uint64) error {
	payload := map[string]interface{}{
		"user_id": user_id,
		"role_id": role_id,
		"chat_id": chat_id,
	}
	var resp interface{}
	return apiClient.PostJSON(ctx, "/api/members/set-role", payload, &resp)
}

// CloneRole клонирует роль через HTTP
func CloneRole(ctx context.Context, apiClient *services.HTTPService, sourceRoleID uint64, targetChatID uint64) error {
	payload := map[string]interface{}{
		"source_role_id": sourceRoleID,
		"target_chat_id": targetChatID,
	}
	var resp interface{}
	return apiClient.PostJSON(ctx, "/api/roles/clone", payload, &resp)
}

// InitDefaultRoles инициализирует стандартные роли в чате через HTTP
func InitDefaultRoles(ctx context.Context, apiClient *services.HTTPService, chat_id uint64) error {
	endpoint := fmt.Sprintf("/api/chats/%d/roles/init-defaults", chat_id)
	var resp interface{}
	return apiClient.PostJSON(ctx, endpoint, nil, &resp)
}

// AddMemberRole добавляет роль участнику по базовому короткому имени через HTTP
func AddMemberRole(ctx context.Context, apiClient *services.HTTPService, user_id uint64, baseShort string, chat_id uint64) error {
	payload := map[string]interface{}{
		"user_id":    user_id,
		"base_short": baseShort,
		"chat_id":    chat_id,
	}
	var resp interface{}
	err := apiClient.PostJSON(ctx, "/api/members/add-role", payload, &resp)
	if err != nil {
		return err
	}

	// Обновляем кеш пользователя
	updatedUser, err := GetUserByID(ctx, apiClient, user_id)
	if err == nil {
		UpdateUserCache(ctx, apiClient, updatedUser)
	}

	return nil
}

// SetMemberRole устанавливает роль пользователю в конкретном чате через HTTP
func SetMemberRole(ctx context.Context, apiClient *services.HTTPService, userID uint64, roleID uint64, chatID uint64) error {
	err := SetRole(ctx, apiClient, userID, roleID, chatID)
	if err != nil {
		return err
	}

	user, err := GetUserByID(ctx, apiClient, userID)
	if err == nil {
		UpdateUserCache(ctx, apiClient, user)
	}

	return nil
}

// CreateRole создает новую роль через HTTP
func CreateRole(ctx context.Context, apiClient *services.HTTPService, name string, short_name string, base_short string, chat_id uint64, priority int) error {
	payload := map[string]interface{}{
		"name":       name,
		"short_name": short_name,
		"base_short": base_short,
		"chat_id":    chat_id,
		"priority":   priority,
	}
	var resp interface{}
	return apiClient.PostJSON(ctx, "/api/roles", payload, &resp)
}

// GetRoleByID возвращает роль по ID через HTTP
func GetRoleByID(ctx context.Context, apiClient *services.HTTPService, roleID uint64, chat_id uint64) (structs.Role, error) {
	queryParams := url.Values{}
	queryParams.Set("chat_id", strconv.FormatUint(chat_id, 10))

	var role structs.Role
	endpoint := fmt.Sprintf("/api/roles/%d", roleID)
	err := apiClient.Get(ctx, endpoint, queryParams, &role)
	return role, err
}

// GetRoleByBaseShort возвращает роль по base_short через HTTP
func GetRoleByBaseShort(ctx context.Context, apiClient *services.HTTPService, short_name string, chat_id uint64) (structs.Role, error) {
	queryParams := url.Values{}
	queryParams.Set("chat_id", strconv.FormatUint(chat_id, 10))
	queryParams.Set("base_short", short_name)

	var role structs.Role
	err := apiClient.Get(ctx, "/api/roles/by-base-short", queryParams, &role)
	return role, err
}

// GetRoleByShortName возвращает роль по short_name через HTTP
func GetRoleByShortName(ctx context.Context, apiClient *services.HTTPService, short_name string, chat_id uint64) (structs.Role, error) {
	queryParams := url.Values{}
	queryParams.Set("chat_id", strconv.FormatUint(chat_id, 10))
	queryParams.Set("short_name", short_name)

	var role structs.Role
	err := apiClient.Get(ctx, "/api/roles/by-short-name", queryParams, &role)
	return role, err
}

// GetUsersByRoleID возвращает пользователей по ID роли через HTTP
func GetUsersByRoleID(ctx context.Context, apiClient *services.HTTPService, role_id uint64, chat_id uint64) ([]structs.User, error) {
	return GetUsersByRole(ctx, apiClient, role_id, chat_id)
}

// GetAdminsUsers возвращает всех администраторов чата через HTTP
func GetAdminsUsers(ctx context.Context, apiClient *services.HTTPService, chat_id uint64) ([]structs.User, error) {
	role, err := GetRoleByBaseShort(ctx, apiClient, "admin", chat_id)
	if err != nil {
		return nil, err
	}
	return GetUsersByRoleID(ctx, apiClient, role.ID, chat_id)
}

// GetModeratorsUsers возвращает всех модераторов чата через HTTP
func GetModeratorsUsers(ctx context.Context, apiClient *services.HTTPService, chat_id uint64) ([]structs.User, error) {
	role, err := GetRoleByBaseShort(ctx, apiClient, "moderator", chat_id)
	if err != nil {
		return nil, err
	}
	return GetUsersByRoleID(ctx, apiClient, role.ID, chat_id)
}

// RemoveRole сбрасывает роль пользователя до стандартной "member" через HTTP
func RemoveRole(ctx context.Context, apiClient *services.HTTPService, user_id uint64, chat_id uint64) error {
	role, err := GetRoleByShortName(ctx, apiClient, "member", chat_id)
	if err != nil {
		return err
	}

	return SetRole(ctx, apiClient, user_id, role.ID, chat_id)
}

// DeleteRoleByID удаляет роль по ID через HTTP
func DeleteRoleByID(ctx context.Context, apiClient *services.HTTPService, roleID uint64, chat_id uint64) error {
	queryParams := url.Values{}
	queryParams.Set("chat_id", strconv.FormatUint(chat_id, 10))

	var resp interface{}
	endpoint := fmt.Sprintf("/api/roles/%d", roleID)
	fullEndpoint := endpoint + "?" + queryParams.Encode()
	
	return apiClient.Delete(ctx, fullEndpoint, nil, &resp)
}

// GetMemberRole возвращает текущую роль участника в чате через HTTP
func GetMemberRole(ctx context.Context, apiClient *services.HTTPService, userID uint64, chatID uint64) (structs.Role, error) {
	queryParams := url.Values{}
	queryParams.Set("user_id", strconv.FormatUint(userID, 10))
	queryParams.Set("chat_id", strconv.FormatUint(chatID, 10))

	var role structs.Role
	err := apiClient.Get(ctx, "/api/members/role", queryParams, &role)
	return role, err
}