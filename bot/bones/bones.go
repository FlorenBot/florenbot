package bones

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	std_helpers "florenbot/bot/helpers"
	"florenbot/services"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type bonesClientResponse struct {
	OK   bool `json:"ok"`
	Code string `json:"code"`
	Data struct {
		Balance float64 `json:"balance"`
		Bet     float64 `json:"bet"`
		MinBet  float64 `json:"min_bet"`
		Outcome string  `json:"outcome"`
		Delta   float64 `json:"delta"`
	} `json:"data"`
	Error string `json:"error"`
}

func getBonesBackendBaseURL() string {
	for _, key := range []string{"API_URL", "BACKEND_URL", "FLORENBOT_BACKEND_URL"} {
		if val := strings.TrimSpace(os.Getenv(key)); val != "" {
			return strings.TrimRight(val, "/")
		}
	}
	return "http://127.0.0.1:8080"
}

func doBonesJSONRequest(
	ctx context.Context,
	httpSvc *services.HTTPService,
	method string,
	path string,
	payload any,
) (*bonesClientResponse, error) {
	if httpSvc == nil {
		return nil, fmt.Errorf("http service is nil")
	}

	endpoint := getBonesBackendBaseURL() + path
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

	var parsed bonesClientResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode response (status %d): %w", resp.StatusCode, err)
	}
	return &parsed, nil
}

// HandleBones обрабатывает команду /bones [ставка]
func HandleBones(bot *tgbotapi.BotAPI, message *tgbotapi.Message, httpSvc *services.HTTPService) {
	args := strings.TrimSpace(message.CommandArguments())
	bet, err := strconv.ParseFloat(args, 64)
	if err != nil || bet <= 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Укажите корректную ставку. Пример: `/bones 100`")
		msg.ReplyToMessageID = message.MessageID
		if _, sendErr := bot.Send(msg); sendErr != nil {
			log.Printf("Ошибка отправки уведомления: %v", sendErr)
		}
		return
	}

	userID := uint64(message.From.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	checkResp, err := doBonesJSONRequest(ctx, httpSvc, http.MethodPost, "/api/v1/games/bones/check", map[string]any{
		"user_id":  userID,
		"username": message.From.UserName,
		"bet":      bet,
	})
	if err != nil {
		log.Printf("Ошибка проверки ставки в /bones: %v", err)
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось проверить ваш баланс.")
		if _, sendErr := bot.Send(msg); sendErr != nil {
			log.Printf("Ошибка отправки уведомления: %v", sendErr)
		}
		return
	}

	if !checkResp.OK {
		switch checkResp.Code {
		case "INSUFFICIENT_FUNDS":
			msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("❌ У вас недостаточно монет! Ваш баланс: %.2f", checkResp.Data.Balance))
			if _, sendErr := bot.Send(msg); sendErr != nil {
				log.Printf("Ошибка отправки уведомления: %v", sendErr)
			}
			return
		case "MIN_BET":
			msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("❌ Минимальное число ставки - %.0f", checkResp.Data.MinBet))
			if _, sendErr := bot.Send(msg); sendErr != nil {
				log.Printf("Ошибка отправки уведомления: %v", sendErr)
			}
			return
		case "INVALID_BET":
			msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Укажите корректную ставку. Пример: `/bones 100`")
			msg.ReplyToMessageID = message.MessageID
			if _, sendErr := bot.Send(msg); sendErr != nil {
				log.Printf("Ошибка отправки уведомления: %v", sendErr)
			}
			return
		default:
			msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось проверить ваш баланс.")
			if _, sendErr := bot.Send(msg); sendErr != nil {
				log.Printf("Ошибка отправки уведомления: %v", sendErr)
			}
			return
		}
	}

	// 1. Бросок игрока
	msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("%s бросает кости...", std_helpers.EscapeMarkdown(message.From.FirstName)))
	msg.ReplyToMessageID = message.MessageID
	if _, sendErr := bot.Send(msg); sendErr != nil {
		log.Printf("Ошибка отправки уведомления: %v", sendErr)
	}

	playerDiceMsg := tgbotapi.NewDice(message.Chat.ID)
	playerResult, err := bot.Send(playerDiceMsg)
	if err != nil {
		return
	}
	playerValue := playerResult.Dice.Value

	time.Sleep(3 * time.Second)

	// 2. Бросок бота
	msg = tgbotapi.NewMessage(message.Chat.ID, "🤖 Бот бросает кости...")
	msg.ReplyToMessageID = message.MessageID
	if _, sendErr := bot.Send(msg); sendErr != nil {
		log.Printf("Ошибка отправки уведомления: %v", sendErr)
	}

	botDiceMsg := tgbotapi.NewDice(message.Chat.ID)
	botResult, err := bot.Send(botDiceMsg)
	if err != nil {
		return
	}
	botValue := botResult.Dice.Value

	time.Sleep(3 * time.Second)

	// 3. Расчёт и применение результата на бэкенде
	resolveResp, err := doBonesJSONRequest(ctx, httpSvc, http.MethodPost, "/api/v1/games/bones/resolve", map[string]any{
		"user_id":      userID,
		"bet":          bet,
		"player_value": playerValue,
		"bot_value":    botValue,
	})
	if err != nil || !resolveResp.OK {
		log.Printf("Ошибка применения результата /bones: %v", err)
		if resolveResp != nil && resolveResp.Code == "STATS_UPDATE_FAILED" {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка изменения рекордов :("))
			return
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось изменить ваш баланс."))
		return
	}

	resultText := fmt.Sprintf("📊 **Итоги раунда:**\nВы выбросили: %d\nБот выбросил: %d\n\n", playerValue, botValue)
	switch resolveResp.Data.Outcome {
	case "win":
		resultText += fmt.Sprintf(" **Победа!** Вы оказались удачливее бота и выиграли **%.2f монет**!", bet)
	case "lose":
		resultText += fmt.Sprintf(" **Проигрыш.** Бот победил. Вы потеряли **%.2f монет**.", bet)
	default:
		resultText += "🤝 **Ничья!** Силы равны, монеты остаются при вас."
	}

	finalMsg := tgbotapi.NewMessage(message.Chat.ID, resultText)
	finalMsg.ParseMode = "Markdown"
	if _, sendErr := bot.Send(finalMsg); sendErr != nil {
		log.Printf("Ошибка отправки уведомления: %v", sendErr)
	}
}