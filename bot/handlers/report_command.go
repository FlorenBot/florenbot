package handlers

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

	"florenbot/services"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type reportClientResponse struct {
	OK    bool   `json:"ok"`
	Code  string `json:"code"`
	Error string `json:"error"`
}

func doReportJSONRequest(
	ctx context.Context,
	httpSvc *services.HTTPService,
	method string,
	path string,
	query url.Values,
	payload any,
) (*reportClientResponse, error) {
	if httpSvc == nil {
		return nil, fmt.Errorf("http service is nil")
	}

	endpoint := getBackendBaseURL() + path
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

	var parsed reportClientResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode response (status %d): %w", resp.StatusCode, err)
	}
	return &parsed, nil
}

func HandleReport(bot *tgbotapi.BotAPI, message *tgbotapi.Message, httpSvc *services.HTTPService) {
	debugType := GetEnvBool("DEBUG", false)

	userID := uint64(message.From.ID)
	chatID := message.Chat.ID

	args := message.CommandArguments()
	parts := strings.Fields(args)

	var AvailableActions = []string{
		"delete",
		"create",
	}

	msgNoActions := fmt.Sprintf("❌ Неверный формат! Используйте: `/report [%s]`", strings.Join(AvailableActions, "|"))

	if len(parts) < 1 {
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, msgNoActions))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	action := parts[0]

	switch action {
	case "create":
		if debugType {
			log.Printf("Создание отчета %s\n", args)
		}

		if len(parts) < 2 {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, msgNoActions))
			return
		}

		text := strings.Join(parts[1:], " ")
		if debugType {
			log.Printf("Текст отчета: %s\n", text)
		}

		apiResp, err := doReportJSONRequest(ctx, httpSvc, http.MethodPost, "/api/v1/reports", nil, map[string]any{
			"user_id": userID,
			"chat_id": chatID,
			"text":    text,
		})
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "USER_NOT_FOUND":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Пользователь не найден"))
					return
				case "CHECK_FAILED":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось проверить наличие отчета"))
					return
				case "REPORT_EXISTS":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ У тебя уже есть отчет, подожди пж пока он будет обработан"))
					return
				case "CREATE_FAILED":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось создать отчет"))
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось создать отчет"))
			return
		}

		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ок, брат, я тебя понял))"))

	case "delete":
		q := url.Values{
			"user_id": {strconv.FormatUint(userID, 10)},
			"chat_id": {strconv.FormatInt(chatID, 10)},
		}

		apiResp, err := doReportJSONRequest(ctx, httpSvc, http.MethodDelete, "/api/v1/reports", q, nil)
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "USER_NOT_FOUND":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Пользователь не найден"))
					return
				case "FORBIDDEN_ROLE":
					_, _ = bot.Send(tgbotapi.NewMessage(chatID, "❌ У вас нет прав или ошибка проверки."))
					return
				case "NOT_ADMIN_OWNER":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ У вас нет прав, только владелец может удалять отчеты"))
					return
				case "CHECK_FAILED":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось проверить наличие отчета"))
					return
				case "REPORT_NOT_FOUND":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ У тебя нет отчета"))
					return
				case "DELETE_FAILED":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось удалить отчет"))
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось удалить отчет"))
			return
		}

		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ок, брат, я тебя понял))))"))

	default:
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, msgNoActions))
	}
}