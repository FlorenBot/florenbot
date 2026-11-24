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

type clanClientResponse struct {
	OK   bool `json:"ok"`
	Code string `json:"code"`
	Data struct {
		ID           int64   `json:"id"`
		Name         string  `json:"name"`
		OwnerName    string  `json:"owner_name"`
		MemberCount  int     `json:"member_count"`
		Role         string  `json:"role"`
		Filter       string  `json:"filter"`
		Total        int     `json:"total"`
		Count        int     `json:"count"`
		InviteCode   string  `json:"invite_code"`
		ExpiresAt    string  `json:"expires_at"`
		OwnerID      int64   `json:"owner_id"`
		TargetID     uint64  `json:"target_id"`
		Reason       string  `json:"reason"`
		Clans        []struct {
			ID          int64  `json:"ID"`
			Name        string `json:"Name"`
			MemberCount int    `json:"MemberCount"`
		} `json:"clans"`
		Members []struct {
			ID           uint64  `json:"ID"`
			Username     string  `json:"Username"`
			FirstName    string  `json:"FirstName"`
			Balance      float64 `json:"Balance"`
			MessageCount int     `json:"MessageCount"`
		} `json:"members"`
	} `json:"data"`
	Error string `json:"error"`
}

func doClanJSONRequest(
	ctx context.Context,
	httpSvc *services.HTTPService,
	method string,
	path string,
	query url.Values,
	payload any,
) (*clanClientResponse, error) {
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

	var parsed clanClientResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode response (status %d): %w", resp.StatusCode, err)
	}
	return &parsed, nil
}

// HandleClan обрабатывает команду /clan через REST API бэкенда
func HandleClan(bot *tgbotapi.BotAPI, message *tgbotapi.Message, httpSvc *services.HTTPService) {
	args := message.CommandArguments()
	parts := strings.Fields(args)
	var AvailableActions = []string{
		"create", "list", "delete", "update", "join",
		"leave", "createcode", "getcode", "invite",
		"delcode", "revoke", "kick", "add", "ban", "expirecode", "delexpire", "members", "getrole",
	}
	userID := uint64(message.From.ID)
	debugType := GetEnvBool("DEBUG", false)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if len(parts) < 1 {
		q := url.Values{"user_id": {strconv.FormatUint(userID, 10)}}
		apiResp, err := doClanJSONRequest(ctx, httpSvc, http.MethodGet, "/api/v1/clans/me", q, nil)
		if err != nil {
			if debugType {
				log.Printf("Ошибка получения клана: %v", err)
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не состоите в клане"))
			return
		}
		if !apiResp.OK {
			if apiResp.Code == "CLAN_NOT_FOUND" {
				_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Клан не найден"))
				return
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не состоите в клане"))
			return
		}

		info := fmt.Sprintf("✅ *Информация о клане:*\n\n"+
			"🆔 ID: `%d`\n"+
			"🏷 Название: *%s*\n"+
			"👥 Участников: `%d`\n"+
			"👑 Имя владельца: `%s`",
			apiResp.Data.ID, apiResp.Data.Name, apiResp.Data.MemberCount, apiResp.Data.OwnerName)

		msg := tgbotapi.NewMessage(message.Chat.ID, info)
		msg.ParseMode = "Markdown"
		_, _ = bot.Send(msg)
		return
	}

	action := parts[0]

	switch action {
	case "create":
		if len(parts) < 2 {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неверный формат! Используйте: /clan create [имя]"))
			return
		}
		name := strings.Join(parts[1:], " ")
		apiResp, err := doClanJSONRequest(ctx, httpSvc, http.MethodPost, "/api/v1/clans", nil, map[string]any{
			"user_id":    userID,
			"username":   message.From.UserName,
			"first_name": message.From.FirstName,
			"name":       name,
		})
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "ALREADY_IN_CLAN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы уже состоите в клане"))
					return
				case "UNAUTHORIZED":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не авторизованы"))
					return
				case "INSUFFICIENT_FUNDS":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Недостаточно средств. Необходимо 1000 рублей"))
					return
				case "DB_ERROR":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка при обращении к базе данных"))
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось создать клан"))
			return
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("✅ Клан создан: %s", apiResp.Data.Name)))

	case "members":
		subArg := "active"
		if len(parts) > 1 {
			subArg = strings.ToLower(parts[1])
		}
		if subArg != "active" && subArg != "noactive" {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неверный аргумент! Используйте: `/clan members [active/noactive]`"))
			return
		}

		q := url.Values{
			"user_id": {strconv.FormatUint(userID, 10)},
			"filter":  {subArg},
		}
		apiResp, err := doClanJSONRequest(ctx, httpSvc, http.MethodGet, "/api/v1/clans/me/members", q, nil)
		if err != nil || !apiResp.OK {
			if apiResp != nil && apiResp.Code == "NOT_IN_CLAN" {
				_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не состоите в клане"))
				return
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "⚠️ Произошла ошибка при загрузке участников клана."))
			return
		}

		var responseText string
		if subArg == "active" {
			responseText = fmt.Sprintf("👥 **Активные участники клана (всего: %d):**\n\n", apiResp.Data.Total)
			for i, member := range apiResp.Data.Members {
				name := member.Username
				if name == "" {
					name = member.FirstName
				}
				if name == "" {
					name = fmt.Sprintf("ID: %d", member.ID)
				}
				responseText += fmt.Sprintf("%d. 👤 %s — Баланс: `%.2f` 🪙\n", i+1, name, member.Balance)
			}
		} else {
			responseText = "💤 **Проверка неактивных участников клана:**\n\n"
			for _, member := range apiResp.Data.Members {
				name := member.Username
				if name == "" {
					name = member.FirstName
				}
				responseText += fmt.Sprintf("👤 @%s — мало активен (Сообщений: %d)\n", name, member.MessageCount)
			}
			if apiResp.Data.Count == 0 {
				responseText += "✅ Все участники проявляют активность!"
			}
		}

		msg := tgbotapi.NewMessage(message.Chat.ID, responseText)
		msg.ParseMode = tgbotapi.ModeMarkdown
		if _, err := bot.Send(msg); err != nil {
			log.Printf("[Telegram Error] Не удалось отправить список участников: %v", err)
		}

	case "getrole":
		q := url.Values{"user_id": {strconv.FormatUint(userID, 10)}}
		apiResp, err := doClanJSONRequest(ctx, httpSvc, http.MethodGet, "/api/v1/clans/me/role", q, nil)
		if err != nil || !apiResp.OK {
			if apiResp != nil && apiResp.Code == "NOT_IN_CLAN" {
				_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не состоите в клане"))
				return
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Что то пошло не так..."))
			return
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("✅ Ваша роль в клане: %s", apiResp.Data.Role)))

	case "kick":
		if len(parts) < 2 {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неверный формат! Используйте: /clan kick [причина]"))
			return
		}
		reply := message.ReplyToMessage
		if reply == nil {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Пожалуйста, ответьте на сообщение пользователя который собираетесь кикнуть"))
			return
		}
		reason := strings.Join(parts[1:], " ")
		if len(reason) > 100 {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Причина не должна быть длиннее 100 символов"))
			return
		} else if len(reason) < 3 {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Причина не должна быть короче 3 символов"))
			return
		}

		targetID := uint64(reply.From.ID)
		apiResp, err := doClanJSONRequest(ctx, httpSvc, http.MethodPost, "/api/v1/clans/me/kick", nil, map[string]any{
			"user_id":   userID,
			"target_id": targetID,
			"reason":    reason,
		})
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "NOT_IN_CLAN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не состоите в клане"))
					return
				case "FORBIDDEN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ У вас нет прав администратора/владельца клана"))
					return
				case "TARGET_NOT_IN_CLAN":
					m := tgbotapi.NewMessage(message.Chat.ID, "❌ Пользователь не состоит в клане")
					m.ReplyToMessageID = message.MessageID
					_, _ = bot.Send(m)
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось кикнуть пользователя"))
			return
		}

		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("✅ Пользователь %s кикнут из клана", reply.From.FirstName)))
		_, _ = bot.Send(tgbotapi.NewMessage(int64(targetID), fmt.Sprintf("✅ Вы были кикнуты из клана: %s", reason)))

	case "list":
		apiResp, err := doClanJSONRequest(ctx, httpSvc, http.MethodGet, "/api/v1/clans", nil, nil)
		if err != nil || !apiResp.OK {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Что то пошло не так..."))
			return
		}
		if len(apiResp.Data.Clans) == 0 {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Кланов нет в этом боте!"))
			return
		}
		info := "✅ *Список кланов:*\n\n"
		for _, clan := range apiResp.Data.Clans {
			info += fmt.Sprintf("🆔 ID: `%d`\n🏷 Название: *%s*\n👥 Участников: `%d`\n", clan.ID, clan.Name, clan.MemberCount)
		}
		msg := tgbotapi.NewMessage(message.Chat.ID, info)
		msg.ParseMode = "Markdown"
		_, _ = bot.Send(msg)

	case "ban":
		if len(parts) < 2 {
			m := tgbotapi.NewMessage(message.Chat.ID, "❌ Неверный формат! Используйте: /clan ban [причина]")
			m.ReplyToMessageID = message.MessageID
			_, _ = bot.Send(m)
			return
		}
		userReply := message.ReplyToMessage
		if userReply == nil {
			m := tgbotapi.NewMessage(message.Chat.ID, "❌ Пожалуйста, ответьте на сообщение пользователя которого хотите забанить")
			m.ReplyToMessageID = message.MessageID
			_, _ = bot.Send(m)
			return
		}
		reason := strings.Join(parts[1:], " ")
		if len(reason) > 100 {
			m := tgbotapi.NewMessage(message.Chat.ID, "❌ Причина не должна быть длиннее 100 символов")
			m.ReplyToMessageID = message.MessageID
			_, _ = bot.Send(m)
			return
		} else if len(reason) < 3 {
			m := tgbotapi.NewMessage(message.Chat.ID, "❌ Причина не должна быть короче 3 символов")
			m.ReplyToMessageID = message.MessageID
			_, _ = bot.Send(m)
			return
		}

		targetID := uint64(userReply.From.ID)
		apiResp, err := doClanJSONRequest(ctx, httpSvc, http.MethodPost, "/api/v1/clans/me/ban", nil, map[string]any{
			"user_id":   userID,
			"target_id": targetID,
			"reason":    reason,
		})
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "NOT_IN_CLAN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не состоите в клане"))
					return
				case "FORBIDDEN":
					m := tgbotapi.NewMessage(message.Chat.ID, "❌ У вас нет прав администратора/владельца клана")
					m.ReplyToMessageID = message.MessageID
					_, _ = bot.Send(m)
					return
				case "TARGET_NOT_IN_CLAN":
					m := tgbotapi.NewMessage(message.Chat.ID, "❌ Пользователь не состоит в клане")
					m.ReplyToMessageID = message.MessageID
					_, _ = bot.Send(m)
					return
				}
			}
			m := tgbotapi.NewMessage(message.Chat.ID, "❌ Что то пошло не так...")
			m.ReplyToMessageID = message.MessageID
			_, _ = bot.Send(m)
			return
		}

		m := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("✅ Пользователь %s забанен", userReply.From.FirstName))
		m.ReplyToMessageID = message.MessageID
		_, _ = bot.Send(m)

	case "delete":
		q := url.Values{"user_id": {strconv.FormatUint(userID, 10)}}
		apiResp, err := doClanJSONRequest(ctx, httpSvc, http.MethodDelete, "/api/v1/clans/me", q, nil)
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "NOT_IN_CLAN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не состоите в клане"))
					return
				case "NOT_OWNER":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не являетесь владельцем клана"))
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Что то пошло не так..."))
			return
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "✅ Клан удален"))

	case "invite":
		if message.Chat.Type == "supergroup" || message.Chat.Type == "group" {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Команда /clan invite недоступна в группах"))
			return
		}
		if len(parts) < 2 {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неверный формат! Используйте: /clan invite [код]"))
			return
		}
		apiResp, err := doClanJSONRequest(ctx, httpSvc, http.MethodPost, "/api/v1/clans/join", nil, map[string]any{
			"user_id":   userID,
			"chat_type": message.Chat.Type,
			"code":      parts[1],
		})
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "ALREADY_IN_CLAN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы состоите в клане"))
					return
				case "INVALID_INVITE":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Клана с таким кодом не существует"))
					return
				case "BLACKLISTED":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вас заблокировали!"))
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Что то пошло не так..."))
			return
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "✅ Вы присоеденились в клан!"))
		if apiResp.Data.OwnerID != 0 {
			_, _ = bot.Send(tgbotapi.NewMessage(apiResp.Data.OwnerID, fmt.Sprintf("К вам зашел пользователь %s", message.From.FirstName)))
		}

	case "revoke":
		if message.Chat.Type == "supergroup" || message.Chat.Type == "group" {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Команда /clan revoke недоступна в группах"))
			return
		}
		apiResp, err := doClanJSONRequest(ctx, httpSvc, http.MethodPost, "/api/v1/clans/me/invite-code/revoke", nil, map[string]any{
			"user_id":   userID,
			"chat_type": message.Chat.Type,
		})
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "NOT_IN_CLAN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не состоите в клане"))
					return
				case "FORBIDDEN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не владелец/админ клана"))
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Что то пошло не так..."))
			return
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "✅ Старый код приглашения удален"))
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("✅ Новый код: /clan invite %s", apiResp.Data.InviteCode)))

	case "delcode":
		if message.Chat.Type == "supergroup" || message.Chat.Type == "group" {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Команда /clan delcode недоступна в группах"))
			return
		}
		q := url.Values{
			"user_id":   {strconv.FormatUint(userID, 10)},
			"chat_type": {message.Chat.Type},
		}
		apiResp, err := doClanJSONRequest(ctx, httpSvc, http.MethodDelete, "/api/v1/clans/me/invite-code", q, nil)
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "NOT_IN_CLAN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не состоите в клане"))
					return
				case "FORBIDDEN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не владелец/админ клана"))
					return
				case "NO_INVITE_CODE":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ У вас нет кода приглашения"))
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Что то пошло не так..."))
			return
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "✅ Приглашение удалено"))

	case "add":
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "⚠️ Добавление в клан перенесено в REST API (/api/v1/clans/me/members)"))

	case "createcode":
		if message.Chat.Type == "supergroup" || message.Chat.Type == "group" {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Команда /clan createcode недоступна в группах"))
			return
		}
		apiResp, err := doClanJSONRequest(ctx, httpSvc, http.MethodPost, "/api/v1/clans/me/invite-code", nil, map[string]any{
			"user_id":   userID,
			"chat_type": message.Chat.Type,
		})
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "NOT_IN_CLAN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не состоите в клане"))
					return
				case "FORBIDDEN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не владелец/админ клана"))
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Что то пошло не так..."))
			return
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("✅ Ваш код приглашения: `%s`, скиньте его своим участникам: /clan invite %s", apiResp.Data.InviteCode, apiResp.Data.InviteCode)))

	case "expirecode":
		if message.Chat.Type == "supergroup" || message.Chat.Type == "group" {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Команда /clan expirecode недоступна в группах"))
			return
		}
		if len(parts) < 2 {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неверный формат! Используйте: `/clan expirecode [время]`"))
			return
		}
		apiResp, err := doClanJSONRequest(ctx, httpSvc, http.MethodPost, "/api/v1/clans/me/invite-code/expire", nil, map[string]any{
			"user_id":   userID,
			"chat_type": message.Chat.Type,
			"duration":  parts[1],
		})
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "INVALID_DURATION":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Неверное время! Используйте: `/clan expirecode [время]`"))
					return
				case "NOT_IN_CLAN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не состоите в клане"))
					return
				case "FORBIDDEN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не владелец/админ клана"))
					return
				case "NO_INVITE_CODE":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ У клана нет приглашения"))
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Что то пошло не так..."))
			return
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("✅ Приглашение будет удалено через %s\nОтменить: /clan delexpire", apiResp.Data.ExpiresAt)))

	case "delexpire":
		if message.Chat.Type == "supergroup" || message.Chat.Type == "group" {
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Команда /clan delexpire недоступна в группах"))
			return
		}
		q := url.Values{
			"user_id":   {strconv.FormatUint(userID, 10)},
			"chat_type": {message.Chat.Type},
		}
		apiResp, err := doClanJSONRequest(ctx, httpSvc, http.MethodDelete, "/api/v1/clans/me/invite-code/expire", q, nil)
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "NOT_IN_CLAN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не состоите в клане"))
					return
				case "FORBIDDEN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не владелец/админ клана"))
					return
				case "NO_INVITE_CODE":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ У клана нет приглашения"))
					return
				case "NO_ACTIVE_TIMER":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Нет активного таймера. Пожалуйста, инициализируйте его через: /clan expirecode [время]"))
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Что то пошло не так..."))
			return
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "✅ Срок действия приглашения успешно удален"))

	case "leave":
		apiResp, err := doClanJSONRequest(ctx, httpSvc, http.MethodPost, "/api/v1/clans/leave", nil, map[string]any{
			"user_id": userID,
		})
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "NOT_IN_CLAN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не состоите в клане"))
					return
				case "OWNER_CANNOT_LEAVE":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Владелец клана не может выйти из него"))
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Что-то пошло не так при выходе из клана..."))
			return
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "✅ Вы успешно вышли из клана"))
		if apiResp.Data.OwnerID != 0 {
			_, _ = bot.Send(tgbotapi.NewMessage(apiResp.Data.OwnerID, fmt.Sprintf("Пользователь %s вышел из клана", message.From.FirstName)))
		}

	case "getcode":
		q := url.Values{"user_id": {strconv.FormatUint(userID, 10)}}
		apiResp, err := doClanJSONRequest(ctx, httpSvc, http.MethodGet, "/api/v1/clans/me/invite-code", q, nil)
		if err != nil || !apiResp.OK {
			if apiResp != nil {
				switch apiResp.Code {
				case "NOT_IN_CLAN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не состоите в клане"))
					return
				case "FORBIDDEN":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Вы не владелец/админ клана"))
					return
				case "NO_INVITE_CODE":
					_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ У вас нет приглашения"))
					return
				}
			}
			_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Оишбка при получении приглашения"))
			return
		}
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("✅ Ваш код для приглашения: `%s`\nСкиньте его своим участникам: /clan invite %s", apiResp.Data.InviteCode, apiResp.Data.InviteCode)))

	default:
		formattedActions := strings.Join(AvailableActions, ", ")
		_, _ = bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Введите действие\nИспользуйте: /clan create [имя]\nВсе действия: "+formattedActions))
	}
}