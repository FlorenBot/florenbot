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
	consts "florenbot/consts"
	std_helpers "florenbot/bot/helpers"
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

func getBotSignatureSecret() string {
	for _, key := range []string{"INTERNAL_API_SECRET", "API_SIGNATURE_SECRET"} {
		if val := strings.TrimSpace(os.Getenv(key)); val != "" {
			return val
		}
	}
	return ""
}

func signStandardRequest(req *http.Request, body []byte) {
	secret := getBotSignatureSecret()
	if secret == "" {
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

	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Signature", hex.EncodeToString(mac.Sum(nil)))
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
	if apiClient == nil {
		return nil, fmt.Errorf("api client is nil")
	}

	endpoint := getBackendBaseURL() + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var rawBody []byte
	var bodyReader io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		rawBody = b
		bodyReader = bytes.NewReader(rawBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if signed {
		signStandardRequest(req, rawBody)
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

	var parsed standardAPIResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode response (status %d): %w", resp.StatusCode, err)
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
	if err == nil && apiResp.OK {
		if apiResp.Data.RoleName != "" {
			roleName = apiResp.Data.RoleName
		}
		roleShort = apiResp.Data.RoleShort
		isAdmin = apiResp.Data.IsAdmin
		isOwnerOrCreator = apiResp.Data.IsOwnerOrCreator
		isCreator = apiResp.Data.IsCreator
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
	_, _ = bot.Send(msg)
}

func HandleProfilePrivate(userID uint64, apiClient *services.HTTPService, bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	debugType := GetEnvBool("DEBUG", false)
	if debugType {
		log.Printf("Пользователь %s начал игровой клуб", message.From.FirstName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	apiResp, err := doStandardJSONRequestWithAuth(ctx, apiClient, http.MethodPost, "/api/v1/standard/profile/init", nil, map[string]any{
		"user_id":    userID,
		"username":   message.From.UserName,
		"first_name": message.From.FirstName,
	}, true)
	if err != nil || !apiResp.OK {
		if debugType {
			log.Printf("Ошибка при старте профиля: %v", err)
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось получить профиль"))
		return
	}

	if debugType {
		log.Printf("Результат профиля: %+v", apiResp.Data.Profile)
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
	_, _ = bot.Send(msg)
}

func HandleStart(bot *tgbotapi.BotAPI, apiClient *services.HTTPService, message *tgbotapi.Message) {
	chatType := message.Chat.Type
	chatID := message.Chat.ID

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch chatType {
	case "private":
		HandleProfilePrivate(uint64(message.From.ID), apiClient, bot, message)

	case "group", "supergroup":
		apiResp, err := doStandardJSONRequest(ctx, apiClient, http.MethodPost, "/api/v1/standard/start/group", nil, map[string]any{
			"user_id":    uint64(message.From.ID),
			"username":   message.From.UserName,
			"first_name": message.From.FirstName,
			"chat_id":    chatID,
			"chat_title": message.Chat.Title,
		})
		if err != nil {
			log.Printf("Ошибка при регистрации чата: %v", err)
			_, _ = bot.Send(tgbotapi.NewMessage(chatID, "❌ Не удалось инициализировать ваш профиль или чат."))
			return
		}
		if !apiResp.OK {
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

		_, _ = bot.Send(tgbotapi.NewMessage(chatID, "✅ Чат успешно зарегистрирован!\n"+
			"Роли инициализированы! Просмотреть роли можно командой /roles"))

	default:
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неизвестный тип чата."))
	}
}

func HandleInfo(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Информация о боте\n\n"+
		"Версия: %s\n"+
		"Дата обновления: %s\n"+
		"Владелец: %s\n"+
		"GitHub: %s", consts.VERSION, consts.DATE_LAST_UPDATE, consts.OWNER, consts.REPO_URL)))
}

func HandleBonus(bot *tgbotapi.BotAPI, apiClient *services.HTTPService, message *tgbotapi.Message) {
	userID := uint64(message.From.ID)

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
		if apiResp.Code == "ALREADY_CLAIMED" {
			msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Вы уже получили бонус. Попробуйте через %.0f часов.", apiResp.Data.RemainingHours))
			_, _ = bot.Send(msg)
			return
		}
		log.Printf("Ошибка бонуса (%s): %s", apiResp.Code, apiResp.Error)
		return
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Вы получили ежедневный бонус: %d монет! Ваш новый баланс: %.0f", apiResp.Data.BonusAmount, apiResp.Data.NewBalance))
	_, _ = bot.Send(msg)
}

func HandleVip(bot *tgbotapi.BotAPI, apiClient *services.HTTPService, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	userID := uint64(message.From.ID)

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
		log.Printf("Ошибка VIP (%s): %s", apiResp.Code, apiResp.Error)
		return
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
	msgProcessing := tgbotapi.NewMessage(message.Chat.ID, "Обработка....")
	sentMsg, _ := bot.Send(msgProcessing)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	q := url.Values{"limit": {"10"}}
	apiResp, err := doStandardJSONRequest(ctx, apiClient, http.MethodGet, "/api/v1/standard/top/balance", q, nil)
	if err != nil || !apiResp.OK {
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ошибка доступа к базе данных."))
		return
	}

	if len(apiResp.Data.Users) == 0 {
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
	log.Printf("Команда /topmsg вызвана пользователем %d", message.From.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	q := url.Values{"limit": {"10"}}
	apiResp, err := doStandardJSONRequest(ctx, apiClient, http.MethodGet, "/api/v1/standard/top/messages", q, nil)
	if err != nil || !apiResp.OK {
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ошибка доступа к базе данных."))
		return
	}

	if len(apiResp.Data.Users) == 0 {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	q := url.Values{"user_id": {strconv.FormatUint(userID, 10)}}
	apiResp, err := doStandardJSONRequest(ctx, apiClient, http.MethodGet, "/api/v1/standard/balance", q, nil)
	if err != nil || !apiResp.OK {
		log.Printf("Ошибка получения баланса: %v", err)
		return
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