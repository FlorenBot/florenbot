package structs

import (
	"time"
)

// User представляет модель пользователя
type User struct {
	ID                 uint64    `json:"id"`
	Username           string    `json:"username"`
	FirstName          string    `json:"first_name"`
	Balance            float64   `json:"balance"`
	Euro               float64   `json:"euro"`
	ClanID             *uint64   `json:"clan_id"`
	Vip                int       `json:"vip"`
	VipActiveAt        time.Time `json:"vip_active_at"`
	NegativeReputation int       `json:"negative_reputation"`
	PositiveReputation int       `json:"positive_reputation"`
	Losses             int       `json:"losses"`
	Wins               int       `json:"wins"`
	PromoCode          string    `json:"promocode"`
	MessageCount       int64     `json:"message_count"`
	LastBonusAt        time.Time `json:"last_bonus_at"`
}

// Ban структура для блокировок (если используется в других хелперах)
type Ban struct {
	ID     uint64 `json:"id"`
	UserID uint64 `json:"user_id"`
	Reason string `json:"reason"`
}

// ClanMember структура для участников клана
type ClanMember struct {
	UserID uint64 `json:"user_id"`
	ClanID uint64 `json:"clan_id"`
	Role   string `json:"role"`
}

// UserCommandRestriction структура ограничений команд
type UserCommandRestriction struct {
	UserID  uint64 `json:"user_id"`
	ChatID  uint64 `json:"chat_id"`
	Command string `json:"command"`
}