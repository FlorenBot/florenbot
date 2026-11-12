package handlers

import (
	consts "florenbot/consts"
	helpers "florenbot/engine/helpers"
	std_helpers "florenbot/helpers"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"strconv"
	"time"
	"strings"
	structs "florenbot/structs"
	"context"
	services "florenbot/services"

)

func HandleHelp(bot *tgbotapi.BotAPI, apiClient *services.HTTPService, message *tgbotapi.Message) {
	parsed_chat_id := std_helpers.ParseChatID(uint64(message.Chat.ID))
	user_id := uint64(message.From.ID)

	memberRole, err := helpers.GetMemberRole(user_id, uint64(parsed_chat_id))

	roleName := "Гость"
	if err == nil {
		roleName = memberRole.Name
	}

	info := fmt.Sprintf("👋 Привет! Твоя роль: *%s*\n\n", roleName)
	info += "📜 *Общие команды:*\n"
	info += "/help — Список команд\n"
	info += "/top — Топ сообщений\n"
	info += "/clan - Управление кланами\n"
	info += "/promo - Управление промокодами\n"

	// 1. Команды для Модератора
	if memberRole.ShortName == "moderator" || std_helpers.IsUserAdmin(&memberRole) || std_helpers.IsUserOwnerOrCreator(&memberRole) {
		info += "\n🛡 *Панель модератора:*\n"
		info += "/ban — Ограничить пользователя\n"
		info += "/mute — Запретить общение\n"
		info += "/unmute — Снять мут\n"
	}

	// 2. Команды для Администратора (управление ролями)
	if std_helpers.IsUserAdmin(&memberRole) || std_helpers.IsUserOwnerOrCreator(&memberRole) {
		info += "\n👮 *Панель администратора:*\n"
		info += "/addmoder - Добавить в модерацию\n"
		info += "/admins - Список админов\n"
		info += "/moders - Список модераторов\n"
		info += "/stats - Статистика\n"

	}

	// 3. Команды для Владельца и Создателя
	if std_helpers.IsUserOwnerOrCreator(&memberRole) {
		info += "\n👑 *Панель владельца:*\n"
		info += "/newrole [имя] [короткое] [служебное] — Создать роль\n"
		info += "/addowner - Добавить владельца\n"
		info += "/addadmin - Добавить администратора\n"
		info += "/editrole [ID] [имя] [короткое] — Редактировать роль\n"
		info += "/delrole [ID] — Удалить роль\n"
		info += "/restrict add [command] — Изменить доступ к команде (добавить ограничение)\n"
		info += "/restrict remove [command] — Удалить ограничение\n"
	}

	// 4. Команды только для Создателя
	if std_helpers.IsUserCreator(&memberRole) {
		info += "\n⚙️ *Системные команды (Creator):*\n"
		info += "/sysrole — выдать системную роль\n"
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, info)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func HandleProfilePrivate(user_id uint64, apiClient *services.HTTPService, bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	userProfile, err := helpers.GetOrCreateUser(user_id, message.From.UserName, message.From.FirstName)
	debug_type := GetEnvBool("DEBUG", false)
	if debug_type {
		log.Printf("Пользователь %s начал игровой клуб", message.From.FirstName)
		log.Printf("Результат: %v, %v", userProfile, err)
	}
	
	if err != nil {
		if debug_type {
			log.Printf("Ошибка при старте: %v", err)
		}
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось получить профиль"))
		return
	}

	if userProfile.ID == 0 {
		if debug_type {
			log.Printf("Пользователь %s создается...", message.From.FirstName)
		}
		userProfile, err = helpers.CreateUser(
			user_id,
			message.From.UserName,
			message.From.FirstName,
		)
		if err != nil {
			if debug_type {
				log.Printf("Ошибка при создании пользователя: %v", err)
				return
			}
			bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось создать пользователя"))
			return
		}

		if debug_type {
			log.Printf("Пользователь %s создан", message.From.FirstName)
			log.Printf("Результат: %v", userProfile)
		}
	} else {
		if debug_type {
			log.Printf("Пользователь %s уже существует", message.From.FirstName)
		}
	}

	// Красивое, структурированное приветствие с использованием эмодзи-разделителей
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
		message.From.FirstName, userProfile.Balance, consts.VERSION, consts.DATE_LAST_UPDATE, consts.OWNER)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ParseMode = "Markdown"
	bot.Send(msg)

}



func HandleStart(bot *tgbotapi.BotAPI, apiClient *services.HTTPService, message *tgbotapi.Message) {
	chat_type := message.Chat.Type
	chat_id := message.Chat.ID

	ctx := context.Background()
	switch chat_type {
	case "private":
		// Вызываем функцию для обработки приватного профиля пользователя
		HandleProfilePrivate(uint64(message.From.ID), bot, message)

	case "group", "supergroup":
		// 1. Получаем пользователя из БД или создаем его, если он новый
		user, err := helpers.GetOrCreateUser(uint64(message.From.ID), message.From.UserName, message.From.FirstName)
		if err != nil {
			log.Printf("Ошибка при получении/создании пользователя: %v", err)
			bot.Send(tgbotapi.NewMessage(chat_id, "❌ Не удалось инициализировать ваш профиль."))
			return
		}

		

		// 2. Подготовка ID чата (преобразуем отрицательные ID Telegram в положительные для БД)
		chat_id := message.Chat.ID
		db_id := uint64(chat_id)
		if chat_id < 0 {
			db_id = uint64(-chat_id)
		}

		// Проверяем если у пользователь есть роль creator в беседе
		user_id := user.ID
		ctx := context.Background()
		isCreator, err := std_helpers.IsCreator(ctx, apiClient, chat_id, int64(user_id))
		if err != nil {
			log.Printf("Ошибка при проверке роли creator: %v", err)
		}

		if !isCreator {
			bot.Send(tgbotapi.NewMessage(chat_id, "❌ У вас нет прав для создания чата."))
			return
		}
		// 3. Создаем структуру чата для регистрации
		newChat := structs.Chat{
			ID:     db_id,
			Name:   message.Chat.Title,
			UserID: user.ID, // Привязываем чат к создателю
		}

		// 4. Регистрируем чат в базе данных
		err = helpers.CreateChat(ctx, apiClient, newChat)
		if err != nil {
			log.Printf("Ошибка создания чата: %v", err)
			bot.Send(tgbotapi.NewMessage(chat_id, "❌ Не удалось создать запись чата."))
			return
		}

		// // 5. Инициализируем стандартные роли для этого чата
		err = helpers.InitDefaultRoles(ctx, apiClient, db_id)
		if err != nil {
			log.Printf("Ошибка инициализации ролей: %v", err)
			bot.Send(tgbotapi.NewMessage(chat_id, "❌ Чат зарегистрирован, но произошла ошибка при настройке ролей."))
			return
		}

		err = helpers.AddMemberRole(ctx, apiClient, user.ID, "owner", db_id)

		bot.Send(tgbotapi.NewMessage(chat_id, "✅ Чат успешно зарегистрирован!\n"+
			"Роли инициализированы! Просмотреть роли можно командой /roles"))

	default:
		// Если тип чата не поддерживается
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неизвестный тип чата."))
	}
}

func HandleInfo(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	bot.Send(tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Информация о боте\n\n"+
		"Версия: %s\n"+
		"Дата обновления: %s\n"+
		"Владелец: %s\n"+
		"GitHub: %s", consts.VERSION, consts.DATE_LAST_UPDATE, consts.OWNER, consts.REPO_URL)))
}

func HandleBonus(bot *tgbotapi.BotAPI, apiClient *services.HTTPService, message *tgbotapi.Message) {
    userID := uint64(message.From.ID)
    ctx := context.Background()
    // 1. Получаем время последнего получения бонуса из БД
    lastBonusTime, err := helpers.GetLastBonusTime(ctx, apiClient, userID)
    if err != nil {
        log.Printf("Ошибка получения времени бонуса: %v", err)
        return
    }

    // 2. Проверяем, прошло ли 24 часа
    if time.Since(lastBonusTime).Hours() < 24 {
        remaining := 24 - time.Since(lastBonusTime).Hours()
        msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Вы уже получили бонус. Попробуйте через %.0f часов.", remaining))
        bot.Send(msg)
        return
    }

    // 3. Получаем баланс игрока
    balance, err := helpers.GetUserBalance(ctx, apiClient, userID)
    if err != nil {
        log.Printf("Ошибка получения баланса: %v", err)
        return
    }

    // 4. Начисляем бонус (например, 100 монет)
    bonusAmount := 100
    newBalance := uint64(balance) + uint64(bonusAmount)

    // 5. Обновляем баланс в БД
    err = engine.UpdateBalance(userID, float64(newBalance))
    if err != nil {
        log.Printf("Ошибка обновления баланса: %v", err)
        return
    }

    // 6. Обновляем время последнего получения бонуса
    helpers.UpdateLastBonusTime(userID, time.Now())

    // 7. Сообщаем пользователю об успехе
    msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Вы получили ежедневный бонус: %d монет! Ваш новый баланс: %d", bonusAmount, newBalance))
    bot.Send(msg)
}


func HandleVip(bot *tgbotapi.BotAPI, apiClient *services.HTTPService, message *tgbotapi.Message) {
	chat_id := message.Chat.ID
	user_id := message.From.ID
	ctx := context.Background()
	// 1. Проверяем, является ли пользователь VIP
	is_vip, err := std_helpers.IsUserVIP(ctx, apiClient, uint64(user_id))
	if err != nil {
		log.Printf("Ошибка проверки VIP-статуса: %v", err)
		return
	}

	if is_vip {
		// Если пользователь VIP, отправляем сообщение об этом
		msg := tgbotapi.NewMessage(chat_id, "Вы уже VIP!")
		bot.Send(msg)
		return
	}

	err = helpers.AddVip(uint64(user_id), 1)
	if err != nil {
		log.Printf("Ошибка добавления VIP-статуса: %v", err)
		return
	}

	// 3. Сообщаем пользователю об успехе
	msg := tgbotapi.NewMessage(chat_id, "Вы стали VIP!")
	bot.Send(msg)
}



// func HandleMyAccount(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
// 	chatID := message.Chat.ID

// 	if !message.Chat.IsPrivate() {
// 		bot.Send(tgbotapi.NewMessage(chatID, "Эта команда работает только в личных сообщениях."))
// 		return
// 	}

// 	if message.From == nil {
// 		return
// 	}

// 	userID := message.From.ID

// 	// Ищем счет по owner_id (Telegram ID скрыт внутри базы)
// 	userCheck, err := helpers.GetCheck(uint64(userID))
// 	if err != nil {
// 		bot.Send(tgbotapi.NewMessage(chatID, "У тебя еще нет счета! Создай его с помощью /createcheck"))
// 		return
// 	}

// 	text := fmt.Sprintf(
// 		"💳 **Ваш банковский счет:**\n\n"+
// 			"Имя счета: %s\n"+
// 			"Номер счета: `%s`\n"+ // Пользователь видит только этот безопасный номер
// 			"Баланс: %.2f",
// 		userCheck.AccountName,
// 		userCheck.AccountNumber,
// 		userCheck.Amount,
// 	)

// 	msg := tgbotapi.NewMessage(chatID, text)
// 	msg.ParseMode = "Markdown"
// 	bot.Send(msg)
// }


// 1. Обработчик команды /pay <номер_счета_получателя> <сумма>

// 2. Обработчик команды /createcheck (создание счета для пользователя)
// func HandleCreateCheck(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
// 	chatID := message.Chat.ID

// 	if !message.Chat.IsPrivate() {
// 		bot.Send(tgbotapi.NewMessage(chatID, "Эта команда работает только в личных сообщениях с ботом."))
// 		return
// 	}

// 	if message.From == nil {
// 		bot.Send(tgbotapi.NewMessage(chatID, "Не удалось определить пользователя"))
// 		return
// 	}

// 	userID := message.From.ID

// 	// Проверяем, может у пользователя уже есть счет
// 	_, err := helpers.GetCheck(uint64(userID))
// 	if err == nil {
// 		bot.Send(tgbotapi.NewMessage(chatID, "У вас уже есть созданный счет!"))
// 		return
// 	}

// 	// Логика создания счета в базе данных
// 	err = helpers.CreateDefaultCheck(uint64(userID), message.From.UserName)
// 	if err != nil {
// 		bot.Send(tgbotapi.NewMessage(chatID, "Не удалось создать счет. Попробуйте позже."))
// 		return
// 	}

// 	bot.Send(tgbotapi.NewMessage(chatID, "✅ Ваш счет успешно создан! Теперь вы можете пользоваться переводами."))
// }

// HandlePay обрабатывает команду /pay <номер_счета_получателя> <сумма>
// func HandlePay(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
// 	chatID := message.Chat.ID

// 	// Защита: работает только в личных сообщениях
// 	if !message.Chat.IsPrivate() {
// 		bot.Send(tgbotapi.NewMessage(chatID, "Эта команда работает только в личных сообщениях с ботом."))
// 		return
// 	}

// 	parsedChatID := std_helpers.ParseChatID(uint64(chatID))

// 	if message.From == nil {
// 		bot.Send(tgbotapi.NewMessage(chatID, "Не удалось определить пользователя"))
// 		return
// 	}

// 	userID := message.From.ID

// 	// 1. Проверяем, есть ли счет у отправителя
// 	senderCheck, err := helpers.GetCheck(uint64(userID))
// 	if err != nil {
// 		bot.Send(tgbotapi.NewMessage(chatID, "У тебя нет счета создай - /createcheck"))
// 		return
// 	}

// 	// 2. Разбираем аргументы команды (/pay <номер_счета> <сумма>)
// 	args := strings.Fields(message.CommandArguments())
// 	if len(args) < 2 {
// 		bot.Send(tgbotapi.NewMessage(chatID, "Использование: /pay <номер_счета_получателя> <сумма>"))
// 		return
// 	}

// 	recipientAccountNum := args[0]
// 	amountStr := args[1]

// 	amount, err := strconv.ParseFloat(amountStr, 64)
// 	if err != nil || amount <= 0 {
// 		bot.Send(tgbotapi.NewMessage(chatID, "Неверный формат суммы! Укажите положительное число."))
// 		return
// 	}

// 	// Запрет перевода самому себе
// 	if senderCheck.AccountNumber == recipientAccountNum {
// 		bot.Send(tgbotapi.NewMessage(chatID, "Нельзя переводить средства на свой собственный счет."))
// 		return
// 	}

// 	// 3. Проверяем наличие счета получателя
// 	recipientCheck, err := helpers.GetCheckByAccountNumber(recipientAccountNum)
// 	if err != nil {
// 		bot.Send(tgbotapi.NewMessage(chatID, "У получателя нет счета"))
// 		return
// 	}

// 	// 4. Формируем текст подтверждения
// 	msgText := fmt.Sprintf(
// 		"Хотите перевести на банковский счет пользователя?\n\n"+
// 			"Имя счета получателя: %s\n"+
// 			"Номер счета получателя: %s\n"+
// 			"Имя получателя: %s\n"+
// 			"С счета: %s\n"+
// 			"Сумма: %.2f",
// 		recipientCheck.AccountName,
// 		recipientCheck.AccountNumber,
// 		recipientCheck.RecipientName,
// 		senderCheck.AccountNumber,
// 		amount,
// 	)

// 	// Формируем callback_data с точными параметрами перевода
// 	yesCallbackData := fmt.Sprintf("pay_yes_%s_%.2f", recipientAccountNum, amount)

// 	keyboard := tgbotapi.NewInlineKeyboardMarkup(
// 		tgbotapi.NewInlineKeyboardRow(
// 			tgbotapi.NewInlineKeyboardButtonData("Да", yesCallbackData),
// 			tgbotapi.NewInlineKeyboardButtonData("Нет", "pay_confirm_no"),
// 		),
// 	)

// 	msg := tgbotapi.NewMessage(chatID, msgText)
// 	msg.ReplyMarkup = keyboard

// 	_, err = bot.Send(msg)
// 	if err != nil {
// 		_, _ = parsedChatID, err
// 	}
// }

// HandlePayCallback обрабатывает нажатия инлайн-кнопок подтверждения перевода
// func HandlePayCallback(bot *tgbotapi.BotAPI, callback *tgbotapi.CallbackQuery) {
// 	// Снимаем спиннер загрузки с кнопки
// 	callbackCfg := tgbotapi.NewCallback(callback.ID, "")
// 	_, _ = bot.Request(callbackCfg)

// 	chatID := callback.Message.Chat.ID
// 	messageID := callback.Message.MessageID

// 	// Отмена перевода
// 	if callback.Data == "pay_confirm_no" {
// 		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ Перевод отменен.")
// 		bot.Send(editMsg)
// 		return
// 	}

// 	// Проверяем формат callback_data: "pay_yes_<account_num>_<amount>"
// 	parts := strings.Split(callback.Data, "_")
// 	if len(parts) < 4 || parts[0] != "pay" || parts[1] != "yes" {
// 		return
// 	}

// 	targetAccountNum := parts[2]
// 	amount, err := strconv.ParseFloat(parts[3], 64)
// 	if err != nil || amount <= 0 {
// 		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ Некорректная сумма перевода.")
// 		bot.Send(editMsg)
// 		return
// 	}

// 	senderUserID := uint64(callback.From.ID)

// 	// 1. Получаем счет отправителя
// 	senderCheck, err := helpers.GetCheck(senderUserID)
// 	if err != nil {
// 		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ У вас нет счета.")
// 		bot.Send(editMsg)
// 		return
// 	}

// 	// 2. Получаем счет получателя
// 	targetCheck, err := helpers.GetCheckByAccountNumber(targetAccountNum)
// 	if err != nil {
// 		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ У получателя нет счета.")
// 		bot.Send(editMsg)
// 		return
// 	}

// 	// 3. Проверяем достаточность средств
// 	if senderCheck.Amount < amount {
// 		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, fmt.Sprintf("❌ Недостаточно средств на счете! Ваш баланс: %.2f", senderCheck.Amount))
// 		bot.Send(editMsg)
// 		return
// 	}

// 	// 4. Получаем имя получателя из базы данных
// 	var recipient structs.User
// 	recipientName := fmt.Sprintf("Пользователь #%d", targetCheck.UserID)
// 	if err := engine.DB.Where("id = ?", targetCheck.UserID).First(&recipient).Error; err == nil {
// 		if recipient.FirstName != "" {
// 			recipientName = recipient.FirstName
// 		} else if recipient.Username != "" {
// 			recipientName = recipient.Username
// 		}
// 	}

// 	// 5. Выполняем атомарный перевод средств в БД (транзакция GORM)
// 	err = engine.DB.Transaction(func(tx *gorm.DB) error {
// 		// Списание у отправителя
// 		resSender := tx.Model(&structs.Check{}).
// 			Where("id = ? AND amount >= ?", senderCheck.ID, amount).
// 			Update("amount", gorm.Expr("amount - ?", amount))

// 		if resSender.Error != nil {
// 			return resSender.Error
// 		}
// 		if resSender.RowsAffected == 0 {
// 			return fmt.Errorf("недостаточно средств или счет не найден")
// 		}

// 		// Зачисление получателю
// 		if err := tx.Model(&structs.Check{}).
// 			Where("id = ?", targetCheck.ID).
// 			Update("amount", gorm.Expr("amount + ?", amount)).Error; err != nil {
// 			return err
// 		}

// 		return nil
// 	})

// 	if err != nil {
// 		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ Ошибка проведения транзакции. Попробуйте позже.")
// 		bot.Send(editMsg)
// 		return
// 	}

// 	// 6. Отправляем уведомление получателю перевода
// 	recipientUserID := int64(targetCheck.UserID)
// 	notifyMsgText := fmt.Sprintf(
// 		"🎉 **Вам поступил перевод!**\n\n"+
// 			"Сумма: **%.2f**\n"+
// 			"От кого (номер счета): `%s`",
// 		amount,
// 		senderCheck.AccountNumber,
// 	)

// 	notifyMsg := tgbotapi.NewMessage(recipientUserID, notifyMsgText)
// 	notifyMsg.ParseMode = "Markdown"
// 	_, _ = bot.Send(notifyMsg)

// 	// 7. Формируем и отправляем чек отправителю
// 	receiptText := fmt.Sprintf(
// 		"✅ **Перевод успешно выполнен!**\n\n"+
// 			"**Имя счета получателя:** Основной счет\n"+
// 			"**Номер счета получателя:** %s\n"+
// 			"**Имя получателя:** %s\n"+
// 			"**С счета:** %s\n"+
// 			"**Сумма:** %.2f",
// 		targetCheck.AccountNumber,
// 		recipientName,
// 		senderCheck.AccountNumber,
// 		amount,
// 	)

// 	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, receiptText)
// 	editMsg.ParseMode = "Markdown"
// 	bot.Send(editMsg)
// }

func HandleTopBalance(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {

    // Відправляємо повідомлення про початок обробки
    msgProcessing := tgbotapi.NewMessage(message.Chat.ID, "Обработка....")
    sentMsg, _ := bot.Send(msgProcessing)

    // 1. Отримання даних з БД
    topUsers, err := helpers.GetTopByBalance(10)
    if err != nil {
        bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ошибка доступа к базе данных."))
        return
    }

    if len(topUsers) == 0 {
        bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Список лидеров пока пуст."))
        return
    }

    var builder strings.Builder
    builder.WriteString("🏆 *Топ-10 балансов:*\n\n")

    for i, user := range topUsers {
        name := user.Username
        if name == "" {
            name = user.FirstName
        }
        
        safeName := strings.NewReplacer("_", "\\_", "*", "\\*", "[", "\\[", "`", "\\`").Replace(name)
        
        builder.WriteString(fmt.Sprintf("%d. %s — `%.2f` $\n", i+1, safeName, user.Balance))
    }

    // 2. Редагуємо повідомлення "Обработка..." замість надсилання нового
    editMsg := tgbotapi.NewEditMessageText(message.Chat.ID, sentMsg.MessageID, builder.String())
    editMsg.ParseMode = "Markdown"
    
    _, err = bot.Send(editMsg)
    if err != nil {
        log.Printf("Ошибка при редактировании сообщения: %v", err)
    }
}

func HandleTopMessages(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
    log.Printf("Команда /topmsg вызвана пользователем %d", message.From.ID)

    // 1. Дебаг получения данных из БД
    topUsers, err := helpers.GetTopByMessages(10)
    if err != nil {
        bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ошибка доступа к базе данных."))
        return
    }


    // 2. Дебаг пустого списка
    if len(topUsers) == 0 {
        bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Список лидеров пока пуст."))
        return
    }

    var builder strings.Builder
    builder.WriteString("🏆 **Топ-10 сообщений в чате:**\n\n")

    for i, user := range topUsers {
        name := user.Username
        if name == "" {
            name = user.FirstName
        }
        
        
		builder.WriteString(fmt.Sprintf("%d. %s — %d сообщений\n", i+1, std_helpers.EscapeMarkdown(name), user.MessageCount))
	}

    // 4. Проверка сообщения перед отправкой
    msgText := builder.String()
    log.Printf("Отправка сообщения: %s", msgText)

    msg := tgbotapi.NewMessage(message.Chat.ID, msgText)
    msg.ParseMode = "Markdown"
    
    // 5. Дебаг результата отправки
    _, err = bot.Send(msg)
    if err != nil {
        log.Printf("Ошибка при отправке сообщения ботом: %v", err)
    } else {
        log.Println("Сообщение успешно отправлено")
    }
}

// HandleBalance обрабатывает команду /balance
func HandleBalance(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	user_id := uint64(message.From.ID)
	userProfile, err := helpers.GetUserByID(user_id)
	if err != nil {
		log.Printf("Ошибка получения баланса: %v", err)
		return
	}

	text := fmt.Sprintf("💰 **Ваш текущий баланс:** `%.2f` $.\n"+
		"Играйте в играх /casino, /roulette, /bones. И зарабатывайте деньги.", userProfile.Balance)
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

// HandleProfile обрабатывает команду /profile
func HandleProfile(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	userID := uint64(message.From.ID)
	chatID := uint64(message.Chat.ID)
	parsed_chat_id := std_helpers.ParseChatID(uint64(chatID))

	// 1. Получаем профиль пользователя
	userProfile, err := helpers.GetOrCreateUser(userID, message.From.UserName, message.From.FirstName)
	if err != nil {
		log.Printf("Ошибка профиля: %v", err)
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось получить профиль"))
		return
	}


	chat_title := ""

	if message.Chat.Type == "private" {
		chat_title = ""
	} else {
		chat, err := helpers.GetChatById(parsed_chat_id)
		if err != nil {
			log.Printf("Ошибка чата: %v", err)
			bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось получить чат"))
			return
		}


		if chat == nil {
			chat_title = "Нет чата"
		} else {
			chat_title = chat.Name
		}
	}

	roleName := ""
	if message.Chat.Type == "private" {
		roleName = "Нет роли"
	} else {
		// 2. Получаем роль пользователя в текущем чате
		var member structs.Member
		// Загружаем связь с ролью, чтобы получить её название
		err = engine.DB.Preload("Role").
			Where("user_id = ? AND chat_id = ?", userID, parsed_chat_id).
			First(&member).Error

		roleName = "Нет роли" // Значение по умолчанию
		if err == nil && member.Role.Name != "" {
			roleName = member.Role.Name
		}
	}


	log.Printf("User: %v in Chat: %d, Role: %s", userProfile, parsed_chat_id, roleName)

	// 3. Расчет статуса
	newStatus := 0
	if userProfile.PositiveReputation >= 10 {
		newStatus = 3
	} else if userProfile.PositiveReputation >= 5 {
		newStatus = 2
	} else if userProfile.NegativeReputation >= 100 {
		newStatus = 1
	}

	// Обновляем статус в БД, если он изменился
	if userProfile.Status != newStatus {
		userProfile.Status = newStatus
		engine.DB.Model(&userProfile).Update("status", newStatus)
	}

	// 4. Определение текста статуса
	statusTexts := map[int]string{
		3: "Отлично",
		2: "Хорошо",
		1: "Плохо",
		0: "Неизвестно",
	}
	statusText := statusTexts[userProfile.Status]

	// 5. Расчет статистики
	totalReputation := userProfile.NegativeReputation + userProfile.PositiveReputation
	totalGames := userProfile.Wins + userProfile.Losses
	vipLevel := userProfile.Vip


	vipText := ""

	if vipLevel == 0 {
		vipText = "Нет"
	} else {
		vipText = fmt.Sprintf("%d", vipLevel)
	}


	// 6. Формирование и отправка сообщения
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
		userProfile.FirstName,
		userProfile.Balance,
		userProfile.Euro,
		userProfile.FlorenCoin,
		totalReputation,
		userProfile.PositiveReputation,
		userProfile.NegativeReputation,
		roleName,
		totalGames,
		userProfile.Wins,
		userProfile.Losses,
		statusText,
		vipText,
		chat_title,
	)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func HandleQuit(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {

	kickConfig := tgbotapi.KickChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatID: message.Chat.ID,
			UserID: message.From.ID,
		},
	}

	// Пытаемся кикнуть пользователя
	_, err := bot.Request(kickConfig)
	if err != nil {
		log.Printf("Ошибка кика: %v", err)
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось кикнуть пользователя. Возможно, у бота нет прав или пользователь админ."))
		return
	}

	bot.Send(tgbotapi.NewMessage(message.Chat.ID, "До свидания!"))
}
