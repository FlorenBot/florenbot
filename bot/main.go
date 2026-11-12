package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"florenbot/bones"
	"florenbot/consts"
	services "florenbot/services"
	helpers "florenbot/helpers"
	"florenbot/handlers"
	admin_handlers "florenbot/handlers/admin"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"

	"florenbot/workers"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("env not found, using system environment variables")
	}
	verify := flag.Bool("skip", false, "Пропустить проверки валидации хеша")
	version := flag.Bool("version", false, "Версия бота")
	flag.Parse()

	if *version {
		fmt.Printf("Версия: %s\n"+
			"Floren Bot Telegram\n"+
			"Исходный код: %s\n", consts.VERSION, consts.REPO_URL)
		os.Exit(0)
	}

	if *verify {
		log.Println("⚠️ Пропускаем проверку валидации хеша (режим --skip)")
	} else {
		log.Println("Проверка хеширования...")
		if err := helpers.CheckHashAndGpg(); err != nil {
			panic(err)
		}
	}

	log.Printf("ОС: %s, Архитектура: %s", runtime.GOOS, runtime.GOARCH)

	// 2. Читаем BACKEND_URI из окружения
	backendURI := os.Getenv("BACKEND_URI")
	if backendURI == "" {
		log.Fatal("❌ Ошибка: переменная окружения BACKEND_URI не задана в .env")
	}

	// 3. Инициализируем наш HTTP-сервис с полученным URL
	apiClient := services.NewHTTPService(backendURI, 5*time.Second)
	apiClient.SetHeader("User-Agent", "Go-HTTP-Client")

	time.Sleep(4 * time.Second)

	go workers.RunPromoCleanupWorker()
	go workers.ClanRemoveInviteCodeWorker()

	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatal("❌ Переменная BOT_TOKEN не установлена")
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("❌ Ошибка инициализации бота: %v", err)
	}

	bot.Debug = false
	log.Printf("🤖 Авторизовано как @%s", bot.Self.UserName)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)
	startTime := time.Now()

	go func() {
		for update := range updates {
			// 1. Обработка нажатий на инлайн-кнопки (CallbackQuery)
			if update.CallbackQuery != nil {
				if update.CallbackQuery.Message != nil && update.CallbackQuery.Message.Time().Before(startTime) {
					continue
				}

				if strings.HasPrefix(update.CallbackQuery.Data, "pay_") {
					handlers.HandlePayCallback(bot, update.CallbackQuery)
				}
				continue
			}

			// 2. Обработка обычных сообщений
			if update.Message == nil {
				continue
			}

			if update.Message.Time().Before(startTime) {
				continue
			}

			if update.Message.NewChatMembers != nil {
				// handleNewMembers(bot, update)
				continue
			}


			handleMessage(bot, update.Message)
		}
	}()

	<-quit
	log.Println("🛑 Останавливаю бота...")
}

func handleMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	text := message.Text
	lowerText := strings.ToLower(text)

	if strings.Contains(lowerText, "спасибо") {
		log.Printf("Зафиксирована благодарность от @%s", message.From.UserName)
		handlers.HandleThanks(bot, message)
		return
	}

	if strings.HasPrefix(text, "/") || strings.HasPrefix(text, "!") {
		handleCommands(bot, message)
	}
}

func handleCommands(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	parts := strings.Fields(message.Text)
	if len(parts) == 0 {
		return
	}

	command := strings.ToLower(parts[0][1:])
	// userID := uint64(message.From.ID)
	// chatID := uint64(message.Chat.ID)
	// parsed_chat_id := helpers.ParseChatID(uint64(chatID))

	// ПРОВЕРКА ОГРАНИЧЕНИЙ
	// result, err := helper.IsCommandRestricted(userID, uint64(parsed_chat_id), command)
	// log.Printf("DEBUG: [IsCommandRestricted] Result: %v", result)
	// log.Printf("DEBUG: [IsCommandRestricted] Error: %v", err)

	// if err != nil {
	// 	log.Printf("DEBUG: [IsCommandRestricted] Ошибка БД: %v", err)
	// }

	// if result {
	// 	bot.Send(tgbotapi.NewMessage(int64(chatID), "🚫 Вам запрещено использовать эту команду. Обратитесь к администрации чата."))
	// 	return
	// }

	switch command {
	case "start":
		handlers.HandleStart(bot, message)
	case "balance":
		handlers.HandleBalance(bot, message)
	case "pin":
		admin_handlers.HandlePin(bot, message)
	case "profile":
		handlers.HandleProfile(bot, message)
	case "casino":
		handlers.HandleCasino(bot, message)
	case "clan":
		handlers.HandleClan(bot, message)
	case "roulette":
		handlers.HandleRoulette(bot, message)
	case "check":
		handlers.HandleMyAccount(bot, message)
	case "createcheck":
		handlers.HandleCreateCheck(bot, message)
	case "bones":
		bones.HandleBones(bot, message)
	case "q":
		handlers.HandleQuit(bot, message)
	case "top":
		handlers.HandleTopMessages(bot, message)
	case "topbal":
		handlers.HandleTopBalance(bot, message)
	case "givemoney":
		admin_handlers.HandleGiveMoney(bot, message)
	case "pay":
		handlers.HandlePay(bot, message)
	case "cloneroles":
		admin_handlers.HandleCloneRoles(bot, message)
	case "promo":
		handlers.HandlePromo(bot, message)
	case "info":
		handlers.HandleInfo(bot, message)
	case "msg":
		admin_handlers.HandleSendMessage(bot, message)
	case "bonus":
		handlers.HandleBonus(bot, message)
	case "vip":
		handlers.HandleVip(bot, message)
	case "rep":
		handlers.HandleReputation(bot, message)
	case "report":
		handlers.HandleReport(bot, message)
	case "спасибо":
		handlers.HandleThanks(bot, message)
	case "ban":
		admin_handlers.HandleBan(bot, message)
	case "mute":
		admin_handlers.HandleMute(bot, message)
	case "unmute":
		admin_handlers.HandleUnMute(bot, message)
	case "restrict":
		admin_handlers.HandleRestrictUserCmd(bot, message)
	case "rr":
		admin_handlers.HandleRemoveRole(bot, message)
	case "del":
		admin_handlers.HandleDeleteMessage(bot, message)
	case "kick":
		admin_handlers.HandleKick(bot, message)
	case "staff":
		admin_handlers.HandleStaff(bot, message)
	case "setrole":
		admin_handlers.HandleSetRole(bot, message)
	case "newrole":
		admin_handlers.HandleNewRole(bot, message)
	case "editrole":
		admin_handlers.HandleEditRole(bot, message)
	case "help":
		handlers.HandleHelp(bot, message)
	case "roles":
		admin_handlers.HandleRoles(bot, message)
	case "delrole":
		admin_handlers.HandleDeleteRole(bot, message)
	case "addowner":
		admin_handlers.HandleAddOwner(bot, message)
	case "stats":
		admin_handlers.HandleStats(bot, message)
	case "addadmin":
		admin_handlers.HandleAddAdmin(bot, message)
	case "addmoder":
		admin_handlers.HandleAddModer(bot, message)
	case "moders":
		admin_handlers.HandleModers(bot, message)
	case "admins":
		admin_handlers.HandleAdmins(bot, message)
	default:
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неизвестная команда"))
	}
}

// func handleNewMembers(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
// 	for _, newUser := range update.Message.NewChatMembers {
// 		user_id := uint64(newUser.ID)
// 		chat_id := uint64(update.Message.Chat.ID)

// 		// 1. Проверяем бан
// 		isBanned := helper.IsUserBanned(user_id)
// 		if isBanned {
// 			bot.Request(tgbotapi.BanChatMemberConfig{
// 				ChatMemberConfig: tgbotapi.ChatMemberConfig{
// 					ChatID: int64(chat_id),
// 					UserID: int64(user_id),
// 				},
// 			})
// 			continue
// 		}

// 		// 2. Добавляем или обновляем пользователя в таблице users
// 		user := structs.User{
// 			ID:        user_id,
// 			FirstName: newUser.FirstName,
// 			Username:  newUser.UserName,
// 		}
// 		engine.DB.Save(&user)

// 		// 3. Добавляем запись в таблицу members
// 		member := structs.Member{
// 			ChatID: chat_id,
// 			UserID: user_id,
// 			RoleID: 2,
// 		}

// 		err := engine.DB.Where(structs.Member{ChatID: chat_id, UserID: user_id}).
// 			FirstOrCreate(&member).Error

// 		if err != nil {
// 			log.Printf("Ошибка добавления пользователя в таблицу members: %v", err)
// 		}
// 	}
// }