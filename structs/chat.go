package structs

import "time"

// Chat описывает модель чата/беседы
type Chat struct {
	ID        uint64    `json:"id"`
	Name      string    `json:"name"`
	UserID    uint64    `json:"user_id"` // ID создателя чата
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}