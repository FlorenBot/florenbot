package routes

import (
	"net/http"
	"strconv"
	"strings"

	std_helpers "florenbot/bot/helpers"
)

func parseChatIDParam(r *http.Request) (int64, uint64, error) {
	rawChat := strings.TrimSpace(r.URL.Query().Get("chat_id"))
	if rawChat == "" {
		return 0, 0, strconv.ErrSyntax
	}
	chatID, err := strconv.ParseInt(rawChat, 10, 64)
	if err != nil {
		return 0, 0, err
	}
	parsedID := uint64(std_helpers.ParseChatID(uint64(chatID)))
	return chatID, parsedID, nil
}