package helpers

import (
	"context"
	"fmt"

	"florenbot/services"
	"florenbot/structs"
)

// CreateChat регистрирует новый чат на бэкенде через HTTP
func CreateChat(ctx context.Context, apiClient *services.HTTPService, chat structs.Chat) error {
	var resp interface{}
	return apiClient.PostJSON(ctx, "/api/chats", chat, &resp)
}

// GetChatById получает информацию о чате по его ID через HTTP
func GetChatById(ctx context.Context, apiClient *services.HTTPService, chatID uint64) (*structs.Chat, error) {
	var chat structs.Chat
	endpoint := fmt.Sprintf("/api/chats/%d", chatID)
	
	err := apiClient.Get(ctx, endpoint, nil, &chat)
	if err != nil {
		return nil, err
	}
	
	// Если бэкенд возвращает пустую структуру при отсутствии чата
	if chat.ID == 0 {
		return nil, fmt.Errorf("чат не найден")
	}

	return &chat, nil
}

// GetChatByTelegramID (если требуется) получает чат по Telegram ID с учетом возможных отрицательных чисел
func GetChatByTelegramID(ctx context.Context, apiClient *services.HTTPService, tgChatID int64) (*structs.Chat, error) {
	dbID := uint64(tgChatID)
	if tgChatID < 0 {
		dbID = uint64(-tgChatID)
	}
	return GetChatById(ctx, apiClient, dbID)
}