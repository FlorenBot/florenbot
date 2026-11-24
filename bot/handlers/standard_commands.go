 package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	backend_structs "florenbot/backend/engine/structs"
	std_helpers "florenbot/bot/helpers"
	consts "florenbot/consts"
	services "florenbot/services"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type standardAPIResponse struct {
	OK   bool `json:"ok"`
	Code string `json:"code"`
	Data struct {
		RoleName         string `json:"role_name"`
		RoleShort        string `json:"role_short"`
		IsAdmin          bool   `json:"is_admin"`
		IsOwnerOrCreator bool   `json:"is_owner_or_creator"`
		IsCreator        bool   `json:"is_creator"`

		Profile     backend_structs.User `json:"profile"`
		ChatTitle   string               `json:"chat_title"`
		RoleDisplay string               `json:"role_display"`

		IsCreatorInChat bool                 `json:"is_creator_in_chat"`
		RemainingHours  float64              `json:"remaining_hours"`
		BonusAmount     int                  `json:"bonus_amount"`
		NewBalance      float64              `json:"new_balance"`
		IsVIP           bool                 `json:"is_vip"`
		Users           []backend_structs.User `json:"users"`
	} `json:"data"`
	Error string `json:"error"`
}

func isStdDebug() bool {
	return GetEnvBool("DEBUG", false)
}

func maskSecret(s string) string {
	if s == "" {
		return "<EMPTY>"
	}
	if len(s) <= 4 {
		return "****"
	}
	return s[:2] + strings.Repeat("*", len(s)-4) + s[len(s)-2:]
}

func getBotSignatureSecret() string {
	for _, key := range []string{"APP_KEY", "INTERNAL_API_SECRET", "API_SIGNATURE_SECRET"} {
		if val := strings.TrimSpace(os.Getenv(key)); val != "" {
			if isStdDebug() {
				log.Printf("[DEBUG][SIG] Секрет взят из %s (len=%d, masked=%s)", key, len(val), maskSecret(val))
			}
			return val
		}
	}
	if isStdDebug() {
		log.Printf("[DEBUG][SIG] Ни один секрет (APP_KEY, INTERNAL_API_SECRET, API_SIGNATURE_SECRET) не задан")
	}
	return ""
}

func signStandardRequest(req *http.Request, body []byte) {
	secret := getBotSignatureSecret()
	if secret == "" {
		if isStdDebug() {
			log.Printf("[DEBUG][SIG] Подпись НЕ добавлена: пустой секрет для %s %s", req.Method, req.URL.Path)
		}
		return
	}

	ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write([]byte(strings.ToUpper(req.Method)))
	mac.Write([]byte("."))
	mac.Write([]byte(req.URL.Path))
	mac.Write([]byte("."))
	mac.Write(body)

	sig := hex.EncodeToString(mac.Sum(nil))
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Signature", sig)

	if isStdDebug() {
		log.Printf("[DEBUG][SIG] Подписан запрос %s %s | ts=%s | path=%s | body_len=%d | sig=%s",
			strings.ToUpper(req.Method), req.URL.String(), ts, req.URL.Path, len(body), sig)
	}
}

func doStandardJSONRequestWithAuth(
	ctx context.Context,
	apiClient *services.HTTPService,
	method string,
	path string,
	query url.Values,
	payload any,
	signed bool,
) (*standardAPIResponse, error) {
	dbg := isStdDebug()
	if apiClient == nil {
		if dbg {
			log.Printf("[DEBUG][HTTP] apiClient == nil для %s %s", method, path)
		}
		return nil, fmt.Errorf("api client is nil")
	}

	baseURL := getBackendBaseURL()
	endpoint := baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var rawBody []byte
	var bodyReader io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			if dbg {
				log.Printf("[DEBUG][HTTP] Ошибка маршалинга payload для %s %s: %v", method, path, err)
			}
			return nil, err
		}
		rawBody = b
		bodyReader = bytes.NewReader(rawBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		if dbg {
			log.Printf("[DEBUG][HTTP] Ошибка создания запроса %s %s: %v", method, endpoint, err)
		}
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if signed {
		signStandardRequest(req, rawBody)
	} else if dbg {
		log.Printf("[DEBUG][HTTP] Запрос без подписи: %s %s", method, endpoint)
	}

	if dbg {
		log.Printf("[DEBUG][HTTP] --> %s %s | signed=%v | X-Timestamp=%q | X-Signature=%q | body=%s",
			method, endpoint, signed, req.Header.Get("X-Timestamp"), req.Header.Get("X-Signature"), string(rawBody))
	}

	start := time.Now()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		if dbg {
			log.Printf("[DEBUG][HTTP] <-- Ошибка выполнения %s %s за %v: %v", method, endpoint, elapsed, err)
		}
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		if dbg {
			log.Printf("[DEBUG][HTTP] <-- Ошибка чтения тела ответа (status=%d) за %v: %v", resp.StatusCode, elapsed, err)
		}
		return nil, err
	}

	if dbg {
		log.Printf("[DEBUG][HTTP] <-- %s %s | status=%d | elapsed=%v | raw_body=%s",
			method, endpoint, resp.StatusCode, elapsed, string(raw))
	}

	var parsed standardAPIResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		if dbg {
			log.Printf("[DEBUG][HTTP] Ошибка парсинга JSON (status=%d): %v | raw=%s", resp.StatusCode, err, string(raw))
		}
		return nil, fmt.Errorf("decode response (status %d): %w", resp.StatusCode, err)
	}

	if dbg {
		log.Printf("[DEBUG][HTTP] Распарсен ответ: ok=%v, code=%q, error=%q", parsed.OK, parsed.Code, parsed.Error)
	}
	return &parsed, nil
}

func doStandardJSONRequest(
	ctx context.Context,
	apiClient *services.HTTPService,
	method string,
	path string,
	query url.Values,
	payload any,
) (*standardAPIResponse, error) {
	return doStandardJSONRequestWithAuth(ctx, apiClient, method, path, query, payload, false)
}

func HandleHelp(bot *tgbotapi.BotAPI, apiClient *services.HTTPService, message *tgbotapi.Message) {
	userID := uint64(message.From.ID)
	chatID := message.Chat.ID
	dbg := isStdDebug()

	if dbg {
		log.Printf("[DEBUG][HELP] Вызов /help: user_id=%d, chat_id=%d, chat_type=%s", userID, chatID, message.Chat.Type)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	q := url.Values{
		"user_id": {strconv.FormatUint(userID, 10)},
		"chat_id": {strconv.FormatInt(chatID, 10)},
	}

	roleName := "Гость"
	isAdmin := false
	isOwnerOrCreator := false
	isCreator := false
	roleShort := ""

	apiResp, err := doStandardJSONRequest(ctx, apiClient, http.MethodGet, "/api/v1/standard/help-context", q, nil)
	if err != nil {
		if dbg {
			log.Printf("[DEBUG][HELP] Ошибка запроса help-context: %v", err)
		}
	} else if !apiResp.OK {
		if dbg {
			log.Printf("[DEBUG][HELP] help-context вернул ok=false: code=%q, error=%q", apiResp.Code, apiResp.Error)
		}
	} else {
		if apiResp.Data.RoleName != "" {
			roleName = apiResp.Data.RoleName
		}
		roleShort = apiResp.Data.RoleShort
		isAdmin = apiResp.Data.IsAdmin
		isOwnerOrCreator = apiResp.Data.IsOwnerOrCreator
		isCreator = apiResp.Data.IsCreator
		if dbg {
			log.Printf("[DEBUG][HELP] Контекст ролей: role=%q, short=%q, admin=%v, ownerOrCreator=%v, creator=%v",
				roleName, roleShort, isAdmin, isOwnerOrCreator, isCreator)
		}
	}

	info := fmt.Sprintf("👋 Привет! Твоя роль: *%s*\n\n", roleName)
	info += "📜 *Общие команды:*\n"
	info += "/help — Список команд\n"
	info += "/top — Топ сообщений\n"
	info += "/clan - Управление кланами\n"
	info += "/promo - Управление промокодами\n"

	if roleShort == "moderator" || isAdmin || isOwnerOrCreator {
		info += "\n🛡 *Панель модератора:*\n"
		info += "/ban — Ограничить пользователя\n"
		info += "/mute — Запретить общение\n"
		info += "/unmute — Снять мут\n"
	}

	if isAdmin || isOwnerOrCreator {
		info += "\n👮 *Панель администратора:*\n"
		info += "/addmoder - Добавить в модерацию\n"
		info += "/admins - Список админов\n"
		info += "/moders - Список модераторов\n"
		info += "/stats - Статистика\n"
	}

	if isOwnerOrCreator {
		info += "\n👑 *Панель владельца:*\n"
		info += "/newrole [имя] [короткое] [служебное] — Создать роль\n"
		info += "/addowner - Добавить владельца\n"
		info += "/addadmin - Добавить администратора\n"
		info += "/editrole [ID] [имя] [короткое] — Редактировать роль\n"
		info += "/delrole [ID] — Удалить роль\n"
		info += "/restrict add [command] — Изменить доступ к команде (добавить ограничение)\n"
		info += "/restrict remove [command] — Удалить ограничение\n"
	}

	if isCreator {
		info += "\n⚙️ *Системные команды (Creator):*\n"
		info += "/sysrole — выдать системную роль\n"
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, info)
	msg.ParseMode = "Markdown"
	if _, sendErr := bot.Send(msg); sendErr != nil && dbg {
		log.Printf("[DEBUG][HELP] Ошибка отправки сообщения в Telegram: %v", sendErr)
	}
}

func HandleProfilePrivate(userID uint64, apiClient *services.HTTPService, bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	debugType := isStdDebug()
	if debugType {
		log.Printf("[DEBUG][PROFILE_INIT] Пользователь %s (id=%d, username=%q) начал игровой клуб",
			message.From.FirstName, userID, message.From.UserName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	payload := map[string]any{
		"user_id":    userID,
		"username":   message.From.UserName,
		"first_name": message.From.FirstName,
	}

	apiResp, err := doStandardJSONRequestWithAuth(ctx, apiClient, http.MethodPost, "/api/v1/standard/profile/init", nil, payload, true)
	if err != nil || !apiResp.OK {
		if debugType {
			if apiResp != nil {
				log.Printf("[DEBUG][PROFILE_INIT] Ошибка при старте профиля: err=%v, ok=%v, code=%q, error=%q",
					err, apiResp.OK, apiResp.Code, apiResp.Error)
			} else {
				log.Printf("[DEBUG][PROFILE_INIT] Ошибка при старте профиля: err=%v, apiResp=<nil>", err)
			}
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось получить профиль"))
		return
	}

	if debugType {
		log.Printf("[DEBUG][PROFILE_INIT] Результат профиля: %+v", apiResp.Data.Profile)
	}

	text := fmt.Sprintf("👋 **Приветствуем вас, %s!**\n\n"+
		"Добро пожаловать в игровой клуб! 🎰\n"+
		"Текущий баланс: **`%.2f` $** 💰\n\n"+
		"📋 **Доступные команды:**\n"+
		"🔹 `/balance` — Узнать баланс\n"+
		"🔹 `/profile` — Просмотреть профиль\n"+
		"🔹 `/casino [ставка]` — Слоты\n"+
		"🔹 `/roulette [ставка] [цвет]` — Рулетка\n"+
		"🔹 `/bones [ставка]` — Кости 🎲\n\n"+
		"🔹 `/promo active [code]` — Активировать код\n"+
		"🔹 `/promo create [code] [amount]` — Создать код\n"+
		"🔹 `/promo delete [code]` — Удалить код\n\n"+
		"🔹 `/clan` — Управление кланами\n\n"+
		"Новые команды:\n\n"+
		"🔹 `/report` — Сделать отчет\n\n"+
		"Желаем удачи! 🍀\n\n"+
		"Версия бота: %s\nДата: %s\nВладелец: %s",
		message.From.FirstName, apiResp.Data.Profile.Balance, consts.VERSION, consts.DATE_LAST_UPDATE, consts.OWNER)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ParseMode = "Markdown"
	if _, sendErr := bot.Send(msg); sendErr != nil && debugType {
		log.Printf("[DEBUG][PROFILE_INIT] Ошибка отправки приветствия: %v", sendErr)
	}
}

func HandleStart(bot *tgbotapi.BotAPI, apiClient *services.HTTPService, message *tgbotapi.Message) {
	chatType := message.Chat.Type
	chatID := message.Chat.ID
	dbg := isStdDebug()

	if dbg {
		log.Printf("[DEBUG][START] Команда /start: user_id=%d, chat_id=%d, chat_type=%s, title=%q",
			message.From.ID, chatID, chatType, message.Chat.Title)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch chatType {
	case "private":
		HandleProfilePrivate(uint64(message.From.ID), apiClient, bot, message)

	case "group", "supergroup":
		payload := map[string]any{
			"user_id":    uint64(message.From.ID),
			"username":   message.From.UserName,
			"first_name": message.From.FirstName,
			"chat_id":    chatID,
			"chat_title": message.Chat.Title,
		}
		apiResp, err := doStandardJSONRequestWithAuth(ctx, apiClient, http.MethodPost, "/api/v1/standard/start/group", nil, payload, true)
		if err != nil {
			log.Printf("Ошибка при регистрации чата: %v", err)
			if dbg {
				log.Printf("[DEBUG][START] Детали ошибки start/group: chat_id=%d, err=%v", chatID, err)
			}
			_, _ = bot.Send(tgbotapi.NewMessage(chatID, "❌ Не удалось инициализировать ваш профиль или чат."))
			return
		}
		if !apiResp.OK {
			if dbg {
				log.Printf("[DEBUG][START] start/group вернул ok=false: code=%q, error=%q, chat_id=%d",
					apiResp.Code, apiResp.Error, chatID)
			}
			switch apiResp.Code {
			case "USER_ERROR":
				_, _ = bot.Send(tgbotapi.NewMessage(chatID, "❌ Не удалось инициализировать ваш профиль."))
				return
			case "NOT_CREATOR":
				_, _ = bot.Send(tgbotapi.NewMessage(chatID, "❌ У вас нет прав для создания чата."))
				return
			case "CREATE_CHAT_FAILED":
				_, _ = bot.Send(tgbotapi.NewMessage(chatID, "❌ Не удалось создать запись чата."))
				return
			case "INIT_ROLES_FAILED":
				_, _ = bot.Send(tgbotapi.NewMessage(chatID, "❌ Чат зарегистрирован, но произошла ошибка при настройке ролей."))
				return
			default:
				_, _ = bot.Send(tgbotapi.NewMessage(chatID, "❌ Ошибка при регистрации чата."))
				return
			}
		}

		if dbg {
			log.Printf("[DEBUG][START] Чат %d успешно зарегистрирован", chatID)
		}
		_, _ = bot.Send(tgbotapi.NewMessage(chatID, "✅ Чат успешно зарегистрирован!\n"+
			"Роли инициализированы! Просмотреть роли можно командой /roles"))

	default:
		if dbg {
			log.Printf("[DEBUG][START] Неизвестный тип чата: %q (chat_id=%d)", chatType, chatID)
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неизвестный тип чата."))
	}
}

func HandleInfo(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	if isStdDebug() {
		log.Printf("[DEBUG][INFO] Запрошена информация о боте пользователем %d", message.From.ID)
	}
	_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Информация о боте\n\n"+
		"Версия: %s\n"+
		"Дата обновления: %s\n"+
		"Владелец: %s\n"+
		"GitHub: %s", consts.VERSION, consts.DATE_LAST_UPDATE, consts.OWNER, consts.REPO_URL)))
}

func HandleBonus(bot *tgbotapi.BotAPI, apiClient *services.HTTPService, message *tgbotapi.Message) {
	userID := uint64(message.From.ID)
	dbg := isStdDebug()

	if dbg {
		log.Printf("[DEBUG][BONUS] Запрос бонуса: user_id=%d, chat_id=%d", userID, message.Chat.ID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	apiResp, err := doStandardJSONRequest(ctx, apiClient, http.MethodPost, "/api/v1/standard/bonus", nil, map[string]any{
		"user_id":    userID,
		"username":   message.From.UserName,
		"first_name": message.From.FirstName,
	})
	if err != nil {
		log.Printf("Ошибка получения бонуса: %v", err)
		return
	}
	if !apiResp.OK {
		if dbg {
			log.Printf("[DEBUG][BONUS] ok=false: code=%q, error=%q, remaining_hours=%.2f",
				apiResp.Code, apiResp.Error, apiResp.Data.RemainingHours)
		}
		if apiResp.Code == "ALREADY_CLAIMED" {
			msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Вы уже получили бонус. Попробуйте через %.0f часов.", apiResp.Data.RemainingHours))
			_, _ = bot.Send(msg)
			return
		}
		log.Printf("Ошибка бонуса (%s): %s", apiResp.Code, apiResp.Error)
		return
	}

	if dbg {
		log.Printf("[DEBUG][BONUS] Выдан бонус=%d, новый баланс=%.2f", apiResp.Data.BonusAmount, apiResp.Data.NewBalance)
	}
	msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Вы получили ежедневный бонус: %d монет! Ваш новый баланс: %.0f", apiResp.Data.BonusAmount, apiResp.Data.NewBalance))
	_, _ = bot.Send(msg)
}

func HandleVip(bot *tgbotapi.BotAPI, apiClient *services.HTTPService, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	userID := uint64(message.From.ID)
	dbg := isStdDebug()

	if dbg {
		log.Printf("[DEBUG][VIP] Запрос VIP: user_id=%d, chat_id=%d", userID, chatID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	apiResp, err := doStandardJSONRequest(ctx, apiClient, http.MethodPost, "/api/v1/standard/vip", nil, map[string]any{
		"user_id": userID,
	})
	if err != nil {
		log.Printf("Ошибка VIP-запроса: %v", err)
		return
	}
	if !apiResp.OK {
		if dbg {
			log.Printf("[DEBUG][VIP] ok=false: code=%q, error=%q", apiResp.Code, apiResp.Error)
		}
		log.Printf("Ошибка VIP (%s): %s", apiResp.Code, apiResp.Error)
		return
	}

	if dbg {
		log.Printf("[DEBUG][VIP] Ответ VIP: is_vip=%v", apiResp.Data.IsVIP)
	}
	if apiResp.Data.IsVIP {
		msg := tgbotapi.NewMessage(chatID, "Вы уже VIP!")
		_, _ = bot.Send(msg)
		return
	}

	msg := tgbotapi.NewMessage(chatID, "Вы стали VIP!")
	_, _ = bot.Send(msg)
}

func HandleTopBalance(bot *tgbotapi.BotAPI, apiClient *services.HTTPService, message *tgbotapi.Message) {
	dbg := isStdDebug()
	if dbg {
		log.Printf("[DEBUG][TOP_BAL] Запрос топа балансов от user_id=%d, chat_id=%d", message.From.ID, message.Chat.ID)
	}

	msgProcessing := tgbotapi.NewMessage(message.Chat.ID, "Обработка....")
	sentMsg, _ := bot.Send(msgProcessing)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	q := url.Values{"limit": {"10"}}
	apiResp, err := doStandardJSONRequest(ctx, apiClient, http.MethodGet, "/api/v1/standard/top/balance", q, nil)
	if err != nil || !apiResp.OK {
		if dbg {
			log.Printf("[DEBUG][TOP_BAL] Ошибка: err=%v, ok=%v, code=%q, error=%q",
				err, apiResp != nil && apiResp.OK, func() string {
					if apiResp == nil {
						return ""
					}
					return apiResp.Code
				}(), func() string {
					if apiResp == nil {
						return ""
					}
					return apiResp.Error
				}())
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ошибка доступа к базе данных."))
		return
	}

	if len(apiResp.Data.Users) == 0 {
		if dbg {
			log.Printf("[DEBUG][TOP_BAL] Пустой список пользователей")
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Список лидеров пока пуст."))
		return
	}

	var builder strings.Builder
	builder.WriteString("🏆 *Топ-10 балансов:*\n\n")

	for i, user := range apiResp.Data.Users {
		name := user.Username
		if name == "" {
			name = user.FirstName
		}
		safeName := strings.NewReplacer("_", "\\_", "*", "\\*", "[", "\\[", "`", "\\`").Replace(name)
		builder.WriteString(fmt.Sprintf("%d. %s — `%.2f` $\n", i+1, safeName, user.Balance))
	}

	editMsg := tgbotapi.NewEditMessageText(message.Chat.ID, sentMsg.MessageID, builder.String())
	editMsg.ParseMode = "Markdown"
	if _, err := bot.Send(editMsg); err != nil {
		log.Printf("Ошибка при редактировании сообщения: %v", err)
	}
}

func HandleTopMessages(bot *tgbotapi.BotAPI, apiClient *services.HTTPService, message *tgbotapi.Message) {
	dbg := isStdDebug()
	log.Printf("Команда /topmsg вызвана пользователем %d", message.From.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	q := url.Values{"limit": {"10"}}
	apiResp, err := doStandardJSONRequest(ctx, apiClient, http.MethodGet, "/api/v1/standard/top/messages", q, nil)
	if err != nil || !apiResp.OK {
		if dbg {
			log.Printf("[DEBUG][TOP_MSG] Ошибка: err=%v, apiResp=%+v", err, apiResp)
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ошибка доступа к базе данных."))
		return
	}

	if len(apiResp.Data.Users) == 0 {
		if dbg {
			log.Printf("[DEBUG][TOP_MSG] Пустой список пользователей")
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Список лидеров пока пуст."))
		return
	}

	var builder strings.Builder
	builder.WriteString("🏆 **Топ-10 сообщений в чате:**\n\n")

	for i, user := range apiResp.Data.Users {
		name := user.Username
		if name == "" {
			name = user.FirstName
		}
		builder.WriteString(fmt.Sprintf("%d. %s — %d сообщений\n", i+1, std_helpers.EscapeMarkdown(name), user.MessageCount))
	}

	msgText := builder.String()
	log.Printf("Отправка сообщения: %s", msgText)

	msg := tgbotapi.NewMessage(message.Chat.ID, msgText)
	msg.ParseMode = "Markdown"
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Ошибка при отправке сообщения ботом: %v", err)
	} else {
		log.Println("Сообщение успешно отправлено")
	}
}

// HandleBalance обрабатывает команду /balance
func HandleBalance(bot *tgbotapi.BotAPI, apiClient *services.HTTPService, message *tgbotapi.Message) {
	userID := uint64(message.From.ID)
	dbg := isStdDebug()
	if dbg {
		log.Printf("[DEBUG][BALANCE] Запрос баланса: user_id=%d", userID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	q := url.Values{"user_id": {strconv.FormatUint(userID, 10)}}
	apiResp, err := doStandardJSONRequest(ctx, apiClient, http.MethodGet, "/api/v1/standard/balance", q, nil)
	if err != nil || !apiResp.OK {
		log.Printf("Ошибка получения баланса: %v", err)
		if dbg && apiResp != nil {
			log.Printf("[DEBUG][BALANCE] code=%q, error=%q", apiResp.Code, apiResp.Error)
		}
		return
	}

	if dbg {
		log.Printf("[DEBUG][BALANCE] Баланс user_id=%d: %.2f", userID, apiResp.Data.Profile.Balance)
	}
	text := fmt.Sprintf("💰 **Ваш текущий баланс:** `%.2f` $.\n"+
		"Играйте в играх /casino, /roulette, /bones. И зарабатывайте деньги.", apiResp.Data.Profile.Balance)
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ParseMode = "Markdown"
	_, _ = bot.Send(msg)
}

// HandleProfile обрабатывает команду /profile
func HandleProfile(bot *tgbotapi.BotAPI, apiClient *services.HTTPService, message *tgbotapi.Message) {
	userID := uint64(message.From.ID)
	chatID := message.Chat.ID
	dbg := isStdDebug()

	if dbg {
		log.Printf("[DEBUG][PROFILE] Запрос профиля: user_id=%d, chat_id=%d, chat_type=%s", userID, chatID, message.Chat.Type)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	q := url.Values{
		"user_id":    {strconv.FormatUint(userID, 10)},
		"username":   {message.From.UserName},
		"first_name": {message.From.FirstName},
		"chat_id":    {strconv.FormatInt(chatID, 10)},
		"chat_type":  {message.Chat.Type},
	}

	apiResp, err := doStandardJSONRequest(ctx, apiClient, http.MethodGet, "/api/v1/standard/profile", q, nil)
	if err != nil || !apiResp.OK {
		log.Printf("Ошибка профиля: %v", err)
		if dbg && apiResp != nil {
			log.Printf("[DEBUG][PROFILE] ok=%v, code=%q, error=%q", apiResp.OK, apiResp.Code, apiResp.Error)
		}
		if apiResp != nil && apiResp.Code == "CHAT_ERROR" {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось получить чат"))
			return
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось получить профиль"))
		return
	}

	p := apiResp.Data.Profile
	log.Printf("User profile loaded: %+v", p)

	statusTexts := map[int]string{
		3: "Отлично",
		2: "Хорошо",
		1: "Плохо",
		0: "Неизвестно",
	}
	statusText := statusTexts[p.Status]

	totalReputation := p.NegativeReputation + p.PositiveReputation
	totalGames := p.Wins + p.Losses

	vipText := "Нет"
	if p.Vip != 0 {
		vipText = fmt.Sprintf("%d", p.Vip)
	}

	chatTitle := apiResp.Data.ChatTitle
	if message.Chat.Type == "private" {
		chatTitle = ""
	}

	roleDisplay := apiResp.Data.RoleDisplay
	if message.Chat.Type == "private" {
		roleDisplay = "Нет роли"
	}

	text := fmt.Sprintf("👤 **Профиль `%s`:**\n"+
		"└── **Баланс:** `%.2f` $\n"+
		"└── **Евро:** `%.2f` €\n"+
		"└── **Floren Coin:** `%.2f` монет\n"+
		"└── **Репутация:** `%d` (поз: %d, нег: %d)\n"+
		"└── **Роль:** `%s`\n"+
		"└── **Игры:** %d (Побед: %d, Поражений: %d)\n"+
		"└── **Статус:** %s\n"+
		"└── **VIP:** %s\n"+
		"└── **Имя чата:** %s\n"+
		"Приятной вам игры в FlorenBot!",
		p.FirstName,
		p.Balance,
		p.Euro,
		p.FlorenCoin,
		totalReputation,
		p.PositiveReputation,
		p.NegativeReputation,
		roleDisplay,
		totalGames,
		p.Wins,
		p.Losses,
		statusText,
		vipText,
		chatTitle,
	)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ParseMode = "Markdown"
	_, _ = bot.Send(msg)
}

func HandleQuit(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	if isStdDebug() {
		log.Printf("[DEBUG][QUIT] Запрос выхода/кика: user_id=%d, chat_id=%d", message.From.ID, message.Chat.ID)
	}

	kickConfig := tgbotapi.KickChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatID: message.Chat.ID,
			UserID: message.From.ID,
		},
	}

	_, err := bot.Request(kickConfig)
	if err != nil {
		log.Printf("Ошибка кика: %v", err)
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось кикнуть пользователя. Возможно, у бота нет прав или пользователь админ."))
		return
	}

	_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "До свидания!"))
}