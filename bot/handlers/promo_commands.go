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

	std_helpers "florenbot/bot/helpers"
	"florenbot/services"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Promocode struct {
	Code   string
	Amount int
}

type promoClientResponse struct {
	OK   bool `json:"ok"`
	Code string `json:"code"`
	Data struct {
		Code             string  `json:"code"`
		Amount           float64 `json:"amount"`
		NewBalance       float64 `json:"new_balance"`
		ByCreator        bool    `json:"by_creator"`
		FormattedExpires string  `json:"formatted_expires"`
		TimerExisted     bool    `json:"timer_existed"`
		Promocodes       []struct {
			Code   string  `json:"Code"`
			Amount float64 `json:"Amount"`
		} `json:"promocodes"`
		Stats []struct {
			Code      string `json:"code"`
			Users     int64  `json:"users"`
			CreatedAt string `json:"created_at"`
			UpdatedAt string `json:"updated_at"`
		} `json:"stats"`
	} `json:"data"`
	Error string `json:"error"`
}

func doPromoJSONRequest(
	ctx context.Context,
	httpSvc *services.HTTPService,
	method string,
	path string,
	query url.Values,
	payload any,
) (*promoClientResponse, error) {
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

	var parsed promoClientResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode response (status %d): %w", resp.StatusCode, err)
	}
	return &parsed, nil
}

func HandlePromo(bot *tgbotapi.BotAPI, message *tgbotapi.Message, httpSvc *services.HTTPService) {
	args := message.CommandArguments()
	parts := strings.Fields(args)
	userID := uint64(message.From.ID)
	debugType := std_helpers.GetEnvBool("DEBUG", false)

	var availableActions = []string{"active", "create", "delete", "list", "stats", "delexpire", "expire", "edit"}

	if len(parts) < 1 {
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Введите действие\nИспользуйте: /promo create [код] [сумма]\nАктивация промокода: /promo active [код]\nВсе действия: "+strings.Join(availableActions[:], ", ")))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	action := parts[0]

	switch action {
	case "create":
		if len(parts) < 3 {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неверный формат! Используйте: /promo create [код] [сумма]"))
			return
		}
		code := parts[1]
		amount, err := strconv.Atoi(parts[2])
		if err != nil {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Сумма должна быть числом"))
			return
		}

		apiResp, err := doPromoJSONRequest(ctx, httpSvc, http.MethodPost, "/api/v1/promos", nil, map[string]any{
			"user_id":    userID,
			"username":   message.From.UserName,
			"first_name": message.From.FirstName,
			"code":       code,
			"amount_int": amount,
		})
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "UNAUTHORIZED":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не авторизованы"))
					return
				case "INSUFFICIENT_FUNDS":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Недостаточно средств. Необходимо 2000 $"))
					return
				case "ALREADY_EXISTS":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Промокод уже существует"))
					return
				case "DB_ERROR":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка при запросе к БД"))
					return
				case "CREATE_FAILED":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("❌ Ошибка при создании кода: %s", apiResp.Error)))
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось создать промокод"))
			return
		}

		msg := tgbotapi.NewMessage(message.Chat.ID, "✅ Промокод создан")
		msg.ReplyToMessageID = message.MessageID
		_, _ = bot.Send(msg)

	case "delete":
		if len(parts) < 2 {
			msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Неверный формат! Используйте: /promo delete [код]")
			msg.ReplyToMessageID = message.MessageID
			if _, err := bot.Send(msg); err != nil {
				log.Printf("Ошибка отправки уведомления: %v", err)
			}
			return
		}

		code := parts[1]
		q := url.Values{
			"user_id": {strconv.FormatUint(userID, 10)},
			"chat_id": {strconv.FormatInt(message.Chat.ID, 10)},
		}

		apiResp, err := doPromoJSONRequest(ctx, httpSvc, http.MethodDelete, "/api/v1/promos/"+url.PathEscape(code), q, nil)
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "UNAUTHORIZED":
					msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не авторизованы")
					msg.ReplyToMessageID = message.MessageID
					_, _ = bot.Send(msg)
					return
				case "NOT_FOUND":
					msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Промокод не найден")
					msg.ReplyToMessageID = message.MessageID
					_, _ = bot.Send(msg)
					return
				case "FORBIDDEN":
					msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не можете удалить этот промокод")
					msg.ReplyToMessageID = message.MessageID
					_, _ = bot.Send(msg)
					return
				case "DB_ERROR":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка при запросе к БД"))
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка при удалении промокода"))
			return
		}

		if apiResp.Data.ByCreator {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "✅ Промокод '"+code+"' удален\n"+
				"❌ Промокод аннулирован у всех пользователей.\n"+
				"Кто удалил: Создатель"))
			return
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "✅ Промокод '"+code+"' удален и аннулирован у пользователей."))

	case "active":
		if len(parts) < 2 {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Введите код\nИспользуйте: /promo active [код]"))
			return
		}
		code := parts[1]

		apiResp, err := doPromoJSONRequest(ctx, httpSvc, http.MethodPost, "/api/v1/promos/activate", nil, map[string]any{
			"user_id":    userID,
			"username":   message.From.UserName,
			"first_name": message.From.FirstName,
			"code":       code,
		})
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "UNAUTHORIZED":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не авторизованы"))
					return
				case "ALREADY_ACTIVATED":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ У вас уже есть активированный промокод или ошибка БД"))
					return
				case "NOT_FOUND":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Промокод не найден"))
					return
				case "SELF_PROMO":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Нельзя активировать свой промокод!"))
					return
				case "BALANCE_UPDATE_FAILED":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка при начислении бонуса"))
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка при активации"))
			return
		}

		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("✅ Промокод активирован! Вам начислено +%.2f $. Ваш баланс: %2.f", apiResp.Data.Amount, apiResp.Data.NewBalance)))

	case "list":
		q := url.Values{"user_id": {strconv.FormatUint(userID, 10)}}
		apiResp, err := doPromoJSONRequest(ctx, httpSvc, http.MethodGet, "/api/v1/promos", q, nil)
		if err != nil || !apiResp.OK {
			if debugType {
				log.Printf("Ошибка при получении промокодов: %v", err)
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка при получении промокодов"))
			return
		}

		if len(apiResp.Data.Promocodes) == 0 {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ У вас нет активных промокодов"))
			return
		}

		var sb strings.Builder
		sb.WriteString("📋 **Ваши промокоды:**\n\n")
		for _, p := range apiResp.Data.Promocodes {
			sb.WriteString(fmt.Sprintf("├── `%s` — %2.f $\n", p.Code, p.Amount))
		}

		msg := tgbotapi.NewMessage(message.Chat.ID, sb.String())
		msg.ParseMode = "Markdown"
		_, _ = bot.Send(msg)

	case "edit":
		if len(parts) < 3 {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неверный формат! Используйте: `/promo edit [код] [сумма]`"))
			return
		}
		code := parts[1]
		amount, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка: сумма должна быть числом"))
			return
		}

		apiResp, err := doPromoJSONRequest(ctx, httpSvc, http.MethodPatch, "/api/v1/promos/"+url.PathEscape(code), nil, map[string]any{
			"user_id":    userID,
			"username":   message.From.UserName,
			"first_name": message.From.FirstName,
			"amount":     amount,
		})
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "UNAUTHORIZED":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не авторизованы"))
					return
				case "NOT_FOUND":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Промокод не найден"))
					return
				case "FORBIDDEN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не владелец этого промокода"))
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка при обновлении"))
			return
		}

		msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("✅ Промокод *%s* обновлен!\nНовая сумма: *%.2f*", code, apiResp.Data.Amount))
		msg.ParseMode = "Markdown"
		_, _ = bot.Send(msg)

	case "expire":
		if len(parts) < 3 {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неверный формат!\nИспользуйте: `/promo expire [код] [время]`"))
			return
		}
		code := parts[1]
		timeStr := parts[2]

		apiResp, err := doPromoJSONRequest(ctx, httpSvc, http.MethodPost, "/api/v1/promos/"+url.PathEscape(code)+"/expire", nil, map[string]any{
			"user_id":    userID,
			"username":   message.From.UserName,
			"first_name": message.From.FirstName,
			"duration":   timeStr,
		})
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "UNAUTHORIZED":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не авторизованы"))
					return
				case "NOT_FOUND":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неверный код"))
					return
				case "FORBIDDEN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не владелец этого кода"))
					return
				case "ALREADY_HAS_EXPIRE":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ У этого промокода уже установлен срок действия"))
					return
				case "INVALID_DURATION":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка формата времени: "+timeStr+"\nИспользуйте формат (d, h, m)\nПример: `1h`, `30d`"))
					return
				case "REDIS_ERROR":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка системы таймера"))
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка при обновлении в базе данных"))
			return
		}

		msgText := fmt.Sprintf("✅ Промокод успешно обновлен!\nКод: *%s*\nСрок действия до: *%s*", code, apiResp.Data.FormattedExpires)
		msg := tgbotapi.NewMessage(message.Chat.ID, msgText)
		msg.ParseMode = "Markdown"
		_, _ = bot.Send(msg)

	case "delexpire":
		if len(parts) < 2 {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неверный формат!\nИспользуйте: `/promo delexpire [код]`"))
			return
		}
		code := parts[1]
		q := url.Values{"user_id": {strconv.FormatUint(userID, 10)}}

		apiResp, err := doPromoJSONRequest(ctx, httpSvc, http.MethodDelete, "/api/v1/promos/"+url.PathEscape(code)+"/expire", q, nil)
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "UNAUTHORIZED":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не авторизованы"))
					return
				case "NOT_FOUND":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Промокод не найден"))
					return
				case "FORBIDDEN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не владелец этого кода"))
					return
				case "NO_EXPIRE":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Этот промокод не имеет срока действия"))
					return
				case "REDIS_UNAVAILABLE":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка системы: Redis недоступен"))
					return
				case "REDIS_ERROR":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка при удалении таймера в системе"))
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка при удалении срока в БД"))
			return
		}

		responseText := fmt.Sprintf("✅ Срок действия промокода *%s* успешно удален!", code)
		if !apiResp.Data.TimerExisted {
			responseText += "\n_(Таймер уже был завершен или не существовал)_"
		}
		msg := tgbotapi.NewMessage(message.Chat.ID, responseText)
		msg.ParseMode = "Markdown"
		_, _ = bot.Send(msg)

	case "stats":
		apiResp, err := doPromoJSONRequest(ctx, httpSvc, http.MethodGet, "/api/v1/promos/stats", nil, nil)
		if err != nil || !apiResp.OK {
			if debugType {
				log.Printf("Ошибка при получении статистики: %v", err)
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка при получении статистики"))
			return
		}

		if len(apiResp.Data.Stats) == 0 {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Нет активных промокодов"))
			return
		}

		var report strings.Builder
		report.WriteString("📊 **Статистика промокодов**\n\n")
		report.WriteString("```text\n")
		report.WriteString(fmt.Sprintf("%-10s | %-8s | %-10s | %-10s\n", "Код", "Юзеров", "Создан", "Обновлен"))
		report.WriteString("-----------------------------------------\n")
		for _, item := range apiResp.Data.Stats {
			report.WriteString(fmt.Sprintf("%-10s | %-8d | %-10s | %-10s\n", item.Code, item.Users, item.CreatedAt, item.UpdatedAt))
		}
		report.WriteString("```")

		msg := tgbotapi.NewMessage(message.Chat.ID, report.String())
		msg.ParseMode = "Markdown"
		_, _ = bot.Send(msg)

	default:
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неизвестное действие\nВсе действия: "+strings.Join(availableActions[:], ", ")))
	}
}