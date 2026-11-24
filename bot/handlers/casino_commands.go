package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"florenbot/services"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	helpers_user "florenbot/bot/helpers"

)

var StartRandomValue int

func init() {
	StartRandomValue = rand.Intn(100)
}

func GetEnvBool(key string, defaultValue bool) bool {
	value, exists := os.LookupEnv(key)
	if !exists {
		return defaultValue
	}
	boolValue, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return boolValue
}

type casinoAPIRequest struct {
	UserID    uint64 `json:"user_id"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	Bet       int    `json:"bet"`
	Choice    string `json:"choice,omitempty"`
}

type casinoAPIResponse struct {
	OK    bool `json:"ok"`
	Data  struct {
		Reels       []string `json:"reels"`
		Outcome     string   `json:"outcome"`
		Bet         int      `json:"bet"`
		DisplayWin  int      `json:"display_win"`
		Balance     float64  `json:"balance"`
		Spin        int      `json:"spin"`
		LandedColor string   `json:"landed_color"`
		Choice      string   `json:"choice"`
	} `json:"data"`
	Error string `json:"error"`
	Code  string `json:"code"`
}

func getBackendBaseURL() string {
	baseURL := strings.TrimSpace(os.Getenv("BACKEND_API_URL"))
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("API_URL"))
	}
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	return strings.TrimRight(baseURL, "/")
}

func callCasinoEndpoint(
	ctx context.Context,
	httpSvc *services.HTTPService,
	endpoint string,
	payload casinoAPIRequest,
) (*casinoAPIResponse, error) {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := getBackendBaseURL() + endpoint

	// Если у *services.HTTPService есть собственный метод запроса (например, Do/PostJSON),
	// можно вызвать его здесь. Ниже используется стандартный http.Client с таймаутом
	// при наличии httpSvc.
	if httpSvc == nil {
		return nil, fmt.Errorf("http service is nil")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var parsed casinoAPIResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal response (status %d): %w", resp.StatusCode, err)
	}

	return &parsed, nil
}

func formatCasinoAPIError(resp *casinoAPIResponse) string {
	switch resp.Code {
	case "INVALID_BET", "MIN_BET":
		return "❌ Минимальная ставка - 50 монет"
	case "INSUFFICIENT_FUNDS":
		return fmt.Sprintf("❌ Недостаточно монет! Ваш баланс: %.2f", resp.Data.Balance)
	case "RATE_LIMITED":
		return "❌ Слишком много запросов повторите через 30 секунд"
	case "UNAUTHORIZED":
		return "❌ Вы не авторизованы"
	case "INVALID_CHOICE":
		return "❌ Неверно указан цвет. Выберите один из трех: `красное`, `черное` или `зеленое`"
	default:
		if resp.Error != "" {
			return "❌ " + resp.Error
		}
		return "❌ Ошибка выполнения запроса к серверу"
	}
}

// HandleCasino - Слоты (/casino 100) через REST API бэкенда
func HandleCasino(bot *tgbotapi.BotAPI, message *tgbotapi.Message, httpSvc *services.HTTPService) {
	args := message.CommandArguments()
	bet, err := strconv.Atoi(strings.TrimSpace(args))
	debugType := GetEnvBool("DEBUG", false)

	if err != nil || bet <= 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Укажите корректную сумму ставки числом. Пример: `/casino 100`")
		msg.ReplyToMessageID = message.MessageID
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Ошибка отправки уведомления: %v", err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	apiResp, err := callCasinoEndpoint(ctx, httpSvc, "/api/v1/casino/slots", casinoAPIRequest{
		UserID:    uint64(message.From.ID),
		Username:  message.From.UserName,
		FirstName: message.From.FirstName,
		Bet:       bet,
	})
	if err != nil {
		if debugType {
			log.Printf("Ошибка запроса слотов к REST API: %v", err)
		}
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Сервис казино временно недоступен.")
		msg.ReplyToMessageID = message.MessageID
		if _, sendErr := bot.Send(msg); sendErr != nil {
			log.Printf("Ошибка отправки уведомления: %v", sendErr)
		}
		return
	}

	if !apiResp.OK {
		msg := tgbotapi.NewMessage(message.Chat.ID, formatCasinoAPIError(apiResp))
		if apiResp.Code == "INVALID_BET" || apiResp.Code == "MIN_BET" {
			msg.ReplyToMessageID = message.MessageID
		}
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Ошибка отправки уведомления: %v", err)
		}
		return
	}

	r1, r2, r3 := "🍒", "🍋", "🎰"
	if len(apiResp.Data.Reels) == 3 {
		r1, r2, r3 = apiResp.Data.Reels[0], apiResp.Data.Reels[1], apiResp.Data.Reels[2]
	}

	resultStr := fmt.Sprintf("🎰 Крутим барабаны:\n[ %s | %s | %s ]\n\n", r1, r2, r3)
	switch apiResp.Data.Outcome {
	case "jackpot":
		resultStr += fmt.Sprintf("🎉 ДЖЕКПОТ! Вы выиграли %d монет!", apiResp.Data.DisplayWin)
	case "win":
		resultStr += fmt.Sprintf("💵 Победа! Вы выиграли %d монет!", apiResp.Data.DisplayWin)
	default:
		resultStr += fmt.Sprintf("📉 Проигрыш. Вы потеряли %d монет.", bet)
	}

	if _, err := bot.Send(tgbotapi.NewMessage(message.Chat.ID, resultStr)); err != nil {
		log.Printf("Не удалось отправить сообщение: %v", err)
	}
}

// HandleRoulette - Рулетка (/roulette 100 красное) через REST API бэкенда
func HandleRoulette(bot *tgbotapi.BotAPI, message *tgbotapi.Message, httpSvc *services.HTTPService) {
	args := strings.Fields(message.CommandArguments())
	debugType := helpers_user.GetEnvBool("DEBUG", false)

	if len(args) < 2 {
		if _, err := bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Укажите корректную ставку и цвет. Пример: `/roulette 100 красное`")); err != nil {
			log.Printf("Ошибка отправки уведомления: %v", err)
		}
		return
	}

	bet, err := strconv.Atoi(args[0])
	if err != nil || bet <= 0 {
		if _, err := bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ставка должна быть положительным числом! Пример: `/roulette 50 черное`")); err != nil {
			log.Printf("Ошибка отправки уведомления: %v", err)
		}
		return
	}

	choice := strings.ToLower(args[1])
	if choice != "красное" && choice != "черное" && choice != "зеленое" {
		if _, err := bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неверно указан цвет. Выберите один из трех: `красное`, `черное` или `зеленое`")); err != nil {
			log.Printf("Ошибка отправки уведомления: %v", err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	apiResp, err := callCasinoEndpoint(ctx, httpSvc, "/api/v1/casino/roulette", casinoAPIRequest{
		UserID:    uint64(message.From.ID),
		Username:  message.From.UserName,
		FirstName: message.From.FirstName,
		Bet:       bet,
		Choice:    choice,
	})
	if err != nil {
		if debugType {
			log.Printf("Ошибка запроса рулетки к REST API: %v", err)
		}
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Сервис казино временно недоступен.")
		msg.ReplyToMessageID = message.MessageID
		if _, sendErr := bot.Send(msg); sendErr != nil {
			log.Printf("Ошибка отправки уведомления: %v", sendErr)
		}
		return
	}

	if !apiResp.OK {
		msg := tgbotapi.NewMessage(message.Chat.ID, formatCasinoAPIError(apiResp))
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Ошибка отправки уведомления: %v", err)
		}
		return
	}

	output := fmt.Sprintf("🔮 Шарик остановился на: **%s** (%d)\n\n", apiResp.Data.LandedColor, apiResp.Data.Spin)
	if apiResp.Data.Outcome == "win" {
		output += fmt.Sprintf("🎉 Вы угадали! Выигрыш: +%d монет.", apiResp.Data.DisplayWin)
	} else {
		output += fmt.Sprintf("📉 Не повезло. Ставка потеряна: -%d монет.", bet)
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, output)
	msg.ParseMode = "Markdown"
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Ошибка отправки уведомления: %v", err)
	}
}