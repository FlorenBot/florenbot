package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	std_helpers "florenbot/bot/helpers"
	"florenbot/services"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type moderClientResponse struct {
	OK   bool `json:"ok"`
	Code string `json:"code"`
	Data struct {
		TargetUserID int64  `json:"target_user_id"`
		Username     string `json:"username"`
		FirstName    string `json:"first_name"`
		Reason       string `json:"reason"`
		UntilDate    int64  `json:"until_date"`
		DurationStr  string `json:"duration_str"`
		DisplayName  string `json:"display_name"`
	} `json:"data"`
	Error string `json:"error"`
}

func doModerJSONRequest(
	ctx context.Context,
	httpSvc *services.HTTPService,
	method string,
	path string,
	query url.Values,
	payload any,
) (*moderClientResponse, error) {
	if httpSvc == nil {
		return nil, fmt.Errorf("http service is nil")
	}

	endpoint := std_helpers.GetAPIURL() + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var bodyReader io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var parsed moderClientResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode response (status %d): %w", resp.StatusCode, err)
	}
	return &parsed, nil
}

func resolveActionTarget(
	ctx context.Context,
	bot *tgbotapi.BotAPI,
	message *tgbotapi.Message,
	httpSvc *services.HTTPService,
) (int64, string, string, bool) {
	var replyUserID int64
	var usernameArg string
	var reason string
	args := strings.Fields(message.CommandArguments())

	if message.ReplyToMessage != nil && message.ReplyToMessage.From != nil {
		replyUserID = message.ReplyToMessage.From.ID
		reason = strings.Join(args, " ")
	} else {
		if len(args) < 1 {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка: укажите пользователя через @username или ответом на сообщение."))
			return 0, "", "", false
		}
		usernameArg = args[0]
		if len(args) > 1 {
			reason = strings.Join(args[1:], " ")
		}
	}

	apiResp, err := doModerJSONRequest(ctx, httpSvc, http.MethodPost, "/api/v1/moders/resolve-target", nil, map[string]any{
		"reply_user_id": replyUserID,
		"username_arg":  usernameArg,
		"reason":        reason,
	})
	if err != nil || !apiResp.OK {
		if apiResp != nil {
			switch apiResp.Code {
			case "INVALID_USERNAME":
				_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неверный формат юзернейма."))
				return 0, "", "", false
			case "USER_NOT_FOUND":
				_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Пользователь с таким username не найден в базе."))
				return 0, "", "", false
			case "DB_USER_NOT_FOUND":
				_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Пользователь не найден в базе данных."))
				return 0, "", "", false
			}
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Пользователь не найден в базе данных."))
		return 0, "", "", false
	}

	return apiResp.Data.TargetUserID, apiResp.Data.Username, apiResp.Data.Reason, true
}

// HandleBan обрабатывает команду /ban, поддерживая ответы (reply) и указание @username
func HandleBan(bot *tgbotapi.BotAPI, message *tgbotapi.Message, httpSvc *services.HTTPService) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	targetUserID, targetUsername, reason, ok := resolveActionTarget(ctx, bot, message, httpSvc)
	if !ok {
		return
	}

	banConfig := tgbotapi.BanChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatID: message.Chat.ID,
			UserID: targetUserID,
		},
	}

	if _, err := bot.Request(banConfig); err != nil {
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось забанить: "+err.Error()))
		return
	}

	response := "✅ Пользователь " + targetUsername + " забанен."
	if reason != "" {
		response += "\nПричина: " + reason
	}

	_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, response))
}

func HandleKick(bot *tgbotapi.BotAPI, message *tgbotapi.Message, httpSvc *services.HTTPService) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	targetUserID, targetUsername, reason, ok := resolveActionTarget(ctx, bot, message, httpSvc)
	if !ok {
		return
	}

	_, err := bot.Request(tgbotapi.BanChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{ChatID: message.Chat.ID, UserID: targetUserID},
	})
	if err != nil {
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось кикнуть: "+err.Error()))
		return
	}

	_, _ = bot.Request(tgbotapi.UnbanChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{ChatID: message.Chat.ID, UserID: targetUserID},
		OnlyIfBanned:     true,
	})

	response := "✅ Пользователь " + targetUsername + " был кикнут."
	if reason != "" {
		response += "\nПричина: " + reason
	}

	_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, response))
}

// HandleMute обрабатывает команду /mute @user <duration> <reason>
func HandleMute(bot *tgbotapi.BotAPI, message *tgbotapi.Message, httpSvc *services.HTTPService) {
	var replyUserID int64
	var usernameArg string
	var durationStr string
	var reason string
	args := strings.Fields(message.CommandArguments())

	if message.ReplyToMessage != nil && message.ReplyToMessage.From != nil {
		replyUserID = message.ReplyToMessage.From.ID
		if len(args) < 1 {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка: укажите длительность (например, 1h, 30m)."))
			return
		}
		durationStr = args[0]
		reason = strings.Join(args[1:], " ")
	} else {
		if len(args) < 2 {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка: используйте /mute @user <duration> <reason>"))
			return
		}
		usernameArg = args[0]
		durationStr = args[1]
		reason = strings.Join(args[2:], " ")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	apiResp, err := doModerJSONRequest(ctx, httpSvc, http.MethodPost, "/api/v1/moders/mute/prepare", nil, map[string]any{
		"reply_user_id": replyUserID,
		"username_arg":  usernameArg,
		"duration_str":  durationStr,
		"reason":        reason,
	})
	if err != nil || !apiResp.OK {
		if apiResp != nil {
			switch apiResp.Code {
			case "INVALID_USERNAME":
				_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неверный формат юзернейма."))
				return
			case "INVALID_DURATION":
				_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неверный формат времени (используйте 30m, 1h, 1d)."))
				return
			}
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неверный формат времени (используйте 30m, 1h, 1d)."))
		return
	}

	muteConfig := tgbotapi.RestrictChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatID: message.Chat.ID,
			UserID: apiResp.Data.TargetUserID,
		},
		UntilDate: apiResp.Data.UntilDate,
		Permissions: &tgbotapi.ChatPermissions{
			CanSendMessages: false,
		},
	}

	if _, err := bot.Request(muteConfig); err != nil {
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось ограничить пользователя: "+err.Error()))
		return
	}

	response := "🔇 " + apiResp.Data.DisplayName + " отправлен в мут на " + apiResp.Data.DurationStr + "."
	if apiResp.Data.Reason != "" {
		response += "\nПричина: " + apiResp.Data.Reason
	}

	_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, response))
}

// HandleUnMute обрабатывает команду /unmute @user
func HandleUnMute(bot *tgbotapi.BotAPI, message *tgbotapi.Message, httpSvc *services.HTTPService) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	targetUserID, targetUsername, _, ok := resolveActionTarget(ctx, bot, message, httpSvc)
	if !ok {
		return
	}

	unmuteConfig := tgbotapi.RestrictChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatID: message.Chat.ID,
			UserID: targetUserID,
		},
		Permissions: &tgbotapi.ChatPermissions{
			CanSendMessages:       true,
			CanSendMediaMessages:  true,
			CanSendOtherMessages:  true,
			CanAddWebPagePreviews: true,
		},
	}

	if _, err := bot.Request(unmuteConfig); err != nil {
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось снять мут: "+err.Error()))
		return
	}

	displayName := targetUsername
	if displayName == "" {
		displayName = "Пользователь"
	}

	_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "✅ С пользователя "+displayName+" сняты ограничения."))
}

// HandleDeleteMessage обрабатывает команду /del
func HandleDeleteMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message, httpSvc *services.HTTPService) {
	debugType := std_helpers.GetEnvBool("DEBUG", false)
	chatID := message.Chat.ID

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	q := url.Values{
		"user_id": {strconv.FormatInt(int64(message.From.ID), 10)},
		"chat_id": {strconv.FormatInt(chatID, 10)},
	}
	apiResp, err := doModerJSONRequest(ctx, httpSvc, http.MethodGet, "/api/v1/moders/delete-check", q, nil)
	if err != nil {
		if debugType {
			log.Printf("Ошибка получения юзера/чата: %v", err)
		}
		return
	}
	if !apiResp.OK {
		if apiResp.Code == "CHAT_NOT_FOUND" {
			_, _ = bot.Send(tgbotapi.NewMessage(chatID, "❌ Чат не найден."))
		}
		return
	}

	if message.ReplyToMessage == nil {
		_, _ = bot.Send(tgbotapi.NewMessage(chatID, "❌ Ошибка: используйте эту команду в ответ на сообщение, которое нужно удалить."))
		return
	}

	if _, err := bot.Request(tgbotapi.NewDeleteMessage(chatID, message.ReplyToMessage.MessageID)); err != nil {
		_, _ = bot.Send(tgbotapi.NewMessage(chatID, "❌ Не удалось удалить сообщение: "+err.Error()))
		return
	}

	_, _ = bot.Request(tgbotapi.NewDeleteMessage(chatID, message.MessageID))
}