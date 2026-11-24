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

	"florenbot/backend/engine/cache"
	"florenbot/bot/bones"
	"florenbot/bot/handlers"
	admin_handlers "florenbot/bot/handlers/admin"
	helpers "florenbot/bot/helpers"
	"florenbot/bot/workers"
	"florenbot/consts"
	services "florenbot/services"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
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

	backendURI := os.Getenv("BACKEND_URI")
	if backendURI == "" {
		backendURI = helpers.GetAPIURL()
	}

	apiClient := services.NewHTTPService(backendURI, 10*time.Second)
	apiClient.SetHeader("User-Agent", "Go-HTTP-Client")

	cache.InitCache()
	
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
			if update.CallbackQuery != nil {
				if update.CallbackQuery.Message != nil && update.CallbackQuery.Message.Time().Before(startTime) {
					continue
				}
				continue
			}

			if update.Message == nil {
				continue
			}

			if update.Message.Time().Before(startTime) {
				continue
			}

			if update.Message.NewChatMembers != nil {
				continue
			}

			handleMessage(bot, update.Message, apiClient)
		}
	}()

	<-quit
	log.Println("🛑 Останавливаю бота...")
}

func handleMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message, apiClient *services.HTTPService) {
	text := message.Text
	lowerText := strings.ToLower(text)

	if strings.Contains(lowerText, "спасибо") {
		log.Printf("Зафиксирована благодарность от @%s", message.From.UserName)
		handlers.HandleThanks(bot, message, apiClient)
		return
	}

	if strings.HasPrefix(text, "/") || strings.HasPrefix(text, "!") {
		handleCommands(bot, message, apiClient)
	}
}

func handleCommands(bot *tgbotapi.BotAPI, message *tgbotapi.Message, apiClient *services.HTTPService) {
	parts := strings.Fields(message.Text)
	if len(parts) == 0 {
		return
	}

	command := strings.ToLower(strings.TrimLeft(parts[0], "/!"))
	if command == "" {
		return
	}

	switch command {
	case "start":
		handlers.HandleStart(bot, apiClient, message)
	case "balance":
		handlers.HandleBalance(bot, apiClient, message)
	case "pin":
		admin_handlers.HandlePin(bot, message)
	case "profile":
		handlers.HandleProfile(bot, apiClient, message)
	case "casino":
		handlers.HandleCasino(bot, message, apiClient)
	case "clan":
		handlers.HandleClan(bot, message, apiClient)
	case "roulette":
		handlers.HandleRoulette(bot, message, apiClient)
	case "bones":
		bones.HandleBones(bot, message, apiClient)
	case "q":
		handlers.HandleQuit(bot, message)
	case "top":
		handlers.HandleTopMessages(bot, apiClient, message)
	case "topbal":
		handlers.HandleTopBalance(bot, apiClient, message)
	case "givemoney":
		admin_handlers.HandleGiveMoney(bot, message)
	case "cloneroles":
		admin_handlers.HandleCloneRoles(bot, message)
	case "promo":
		handlers.HandlePromo(bot, message, apiClient)
	case "info":
		handlers.HandleInfo(bot, message)
	case "msg":
		admin_handlers.HandleSendMessage(bot, message)
	case "bonus":
		handlers.HandleBonus(bot, apiClient, message)
	case "vip":
		handlers.HandleVip(bot, apiClient, message)
	case "rep":
		handlers.HandleReputation(bot, message, apiClient)
	case "report":
		handlers.HandleReport(bot, message, apiClient)
	case "спасибо":
		handlers.HandleThanks(bot, message, apiClient)
	case "ban":
		admin_handlers.HandleBan(bot, message, apiClient)
	case "mute":
		admin_handlers.HandleMute(bot, message, apiClient)
	case "unmute":
		admin_handlers.HandleUnMute(bot, message, apiClient)
	case "restrict":
		admin_handlers.HandleRestrictUserCmd(bot, message)
	case "rr":
		admin_handlers.HandleRemoveRole(bot, message)
	case "del":
		admin_handlers.HandleDeleteMessage(bot, message, apiClient)
	case "kick":
		admin_handlers.HandleKick(bot, message, apiClient)
	case "staff":
		admin_handlers.HandleStaff(bot, message)
	case "setrole":
		admin_handlers.HandleSetRole(bot, message)
	case "newrole":
		admin_handlers.HandleNewRole(bot, message)
	case "editrole":
		admin_handlers.HandleEditRole(bot, message)
	case "help":
		handlers.HandleHelp(bot, apiClient, message)
	case "roles":
		admin_handlers.HandleRoles(bot, message)
	case "delrole":
		admin_handlers.HandleDeleteRole(bot, message)
	case "addowner":
		admin_handlers.HandleAddOwner(bot, message)
	case "stats":
		admin_handlers.HandleStats(bot, message,)
	case "addadmin":
		admin_handlers.HandleAddAdmin(bot, message)
	case "addmoder":
		admin_handlers.HandleAddModer(bot, message)
	case "moders":
		admin_handlers.HandleModers(bot, message)
	case "admins":
		admin_handlers.HandleAdmins(bot, message)
	default:
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неизвестная команда"))
	}
}