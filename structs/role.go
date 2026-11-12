package structs

// Role описывает модель роли в определенном чате
type Role struct {
	ID        uint64 `json:"id" gorm:"primaryKey"`
	Name      string `json:"name" gorm:"not null"`
	ShortName string `json:"short_name" gorm:"not null"`
	BaseShort string `json:"base_short" gorm:"not null"` // Например: creator, owner, admin, moderator, member
	ChatID    uint64 `json:"chat_id" gorm:"index"`
	Priority  int    `json:"priority"` // Уровень приоритета (чем выше, тем больше прав)
}

// Member описывает связь пользователя с конкретным чатом и его роль в нем (таблица chat_members)
type Member struct {
	ID     uint64 `json:"id" gorm:"primaryKey"`
	ChatID uint64 `json:"chat_id" gorm:"index;not null"`
	UserID uint64 `json:"user_id" gorm:"index;not null"`
	RoleID uint64 `json:"role_id" gorm:"not null"`
}