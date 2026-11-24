package structs

import "time"

type Check struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	UserID        int64     `gorm:"column:owner_id;index" json:"owner_id"`
	Name          string    `gorm:"column:name" json:"name"`
	AccountName   string    `gorm:"column:account_name" json:"account_name"`     // Имя счета получателя
	AccountNumber string    `gorm:"column:account_number" json:"account_number"` // Номер счета получателя
	RecipientName string    `gorm:"column:recipient_name" json:"recipient_name"` // Имя получателя
	SourceAccount string    `gorm:"column:source_account" json:"source_account"` // С какого счета
	Amount        float64   `gorm:"column:amount" json:"amount"`                 // Сумма перевода
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}