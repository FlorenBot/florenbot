package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	std_helpers "florenbot/bot/helpers"
	"florenbot/services"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type reputateClientResponse struct {
	OK    bool   `json:"ok"`
	Code  string `json:"code"`
	Error string `json:"error"`
}

func doReputateJSONRequest(
	ctx context.Context,
	httpSvc *services.HTTPService,
	method string,
	path string,
	payload any,
) (*reputateClientResponse, error) {
	if httpSvc == nil {
		return nil, fmt.Errorf("http service is nil")
	}

	endpoint := getBackendBaseURL() + path
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

	var parsed reputateClientResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode response (status %d): %w", resp.StatusCode, err)
	}
	return &parsed, nil
}

func HandleThanks(bot *tgbotapi.BotAPI, message *tgbotapi.Message, httpSvc *services.HTTPService) {
	userReply := message.ReplyToMessage
	if userReply == nil || userReply.From == nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Пожалуйста, ответьте на сообщение пользователя.")
		msg.ReplyToMessageID = message.MessageID
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Ошибка отправки уведомления: %v", err)
		}
		return
	}

	replyUserID := uint64(userReply.From.ID)
	debugType := std_helpers.GetEnvBool("DEBUG", false)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	apiResp, err := doReputateJSONRequest(ctx, httpSvc, http.MethodPost, "/api/v1/reputation/thanks", map[string]any{
		"target_user_id": replyUserID,
	})
	if err != nil || !apiResp.OK {
		if debugType {
			log.Printf("Ошибка при обработке благодарности: %v", err)
		}
		errText := "❌ Ошибка обновления репутации в базе"
		if apiResp != nil {
			switch apiResp.Code {
			case "USER_NOT_FOUND":
				errText = "❌ Пользователь не найден"
			case "GET_REP_FAILED":
				errText = "❌ Ошибка получения репутации"
			}
		}
		msg := tgbotapi.NewMessage(message.Chat.ID, errText)
		msg.ReplyToMessageID = message.MessageID
		if _, sendErr := bot.Send(msg); sendErr != nil {
			log.Printf("Ошибка отправки уведомления: %v", sendErr)
		}
		return
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, "И тебе!")
	msg.ReplyToMessageID = message.MessageID
	_, _ = bot.Send(msg)
}

func HandleReputation(bot *tgbotapi.BotAPI, message *tgbotapi.Message, httpSvc *services.HTTPService) {
	availableActions := []string{"полож", "отриц"}

	args := message.CommandArguments()
	parts := strings.Fields(args)

	if message.Chat.Type == "private" {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Пожалуйста, используйте бота в группе.")
		msg.ReplyToMessageID = message.MessageID
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Ошибка отправки уведомления: %v", err)
		}
		return
	}

	if len(parts) < 2 {
		helpText := fmt.Sprintf("❌ Неверный формат! Используйте: `/rep [%s] [количество]`", strings.Join(availableActions, "|"))
		msg := tgbotapi.NewMessage(message.Chat.ID, helpText)
		msg.ReplyToMessageID = message.MessageID
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Ошибка отправки уведомления: %v", err)
		}
		return
	}

	if message.ReplyToMessage == nil || message.ReplyToMessage.From == nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Пожалуйста, ответьте на сообщение пользователя.")
		msg.ReplyToMessageID = message.MessageID
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Ошибка отправки уведомления: %v", err)
		}
		return
	}

	reputate, err := strconv.Atoi(parts[1])
	if err != nil || reputate <= 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Укажите корректное положительное число для изменения репутации.")
		msg.ReplyToMessageID = message.MessageID
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Ошибка отправки уведомления: %v", err)
		}
		return
	}

	action := parts[0]
	if action != "полож" && action != "отриц" {
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Использование: `/rep [%s] [количество]`", strings.Join(availableActions, ","))))
		return
	}

	replyUserID := uint64(message.ReplyToMessage.From.ID)
	actorUserID := uint64(message.From.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	apiResp, err := doReputateJSONRequest(ctx, httpSvc, http.MethodPost, "/api/v1/reputation/change", map[string]any{
		"actor_user_id":  actorUserID,
		"target_user_id": replyUserID,
		"target_name":    message.ReplyToMessage.From.FirstName,
		"action":         action,
		"amount":         reputate,
	})
	if err != nil || !apiResp.OK {
		if apiResp != nil {
			switch apiResp.Code {
			case "UNAUTHORIZED":
				msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не авторизованы (зарегистрируйтесь в боте)")
				msg.ReplyToMessageID = message.MessageID
				_, _ = bot.Send(msg)
				return
			case "SELF_REPUTATION":
				msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не можете изменять репутацию самому себе!")
				msg.ReplyToMessageID = message.MessageID
				_, _ = bot.Send(msg)
				return
			case "TARGET_NOT_FOUND":
				_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Пользователь не найден"))
				return
			case "GET_REP_FAILED":
				_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка получения репутации"))
				return
			case "NO_REPUTATION":
				_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ У пользователя нет репутации для уменьшения"))
				return
			case "UPDATE_REP_FAILED":
				_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка обновления репутации в базе"))
				return
			}
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка обновления репутации в базе"))
		return
	}

	var msgText string
	if action == "отриц" {
		msgText = fmt.Sprintf("✅ Репутация пользователя %s уменьшена на %d", message.ReplyToMessage.From.FirstName, reputate)
	} else {
		msgText = fmt.Sprintf("✅ Репутация пользователя %s увеличена на %d", message.ReplyToMessage.From.FirstName, reputate)
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, msgText)
	msg.ReplyToMessageID = message.MessageID
	_, _ = bot.Send(msg)
}