package helpers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	engine "florenbot/engine/mysql"
	"florenbot/engine/structs"
)

// Генерация случайного публичного номера счета (например: ACC-8f4a1c)
func generateAccountNumber() string {
	bytes := make([]byte, 3) // 3 байта = 6 шестнадцатеричных символов
	_, _ = rand.Read(bytes)
	return fmt.Sprintf("ACC-%s", hex.EncodeToString(bytes))
}

// CreateDefaultCheck создает новый счет с уникальным рандомным номером счета
func CreateDefaultCheck(userID uint64, userName string) error {
	accountNum := generateAccountNumber()

	// Проверяем уникальность сгенерированного номера в БД
	for {
		var existing structs.Check
		err := engine.DB.Where("account_number = ?", accountNum).First(&existing).Error
		if err != nil {
			// Если запись не найдена, значит номер свободен
			break
		}
		accountNum = generateAccountNumber()
	}

	newCheck := structs.Check{
		UserID:        int64(userID),
		Name:          userName,
		AccountName:   "Основной счет",
		AccountNumber: accountNum,
		RecipientName: userName,
		SourceAccount: accountNum,
		Amount:        0.0,
	}

	return engine.DB.Create(&newCheck).Error
}

// GetCheck ищет чек пользователя по owner_id (Telegram ID)
func GetCheck(userID uint64) (*structs.Check, error) {
	var check structs.Check

	err := engine.DB.Where("owner_id = ?", userID).First(&check).Error
	if err != nil {
		return nil, err
	}

	return &check, nil
}

// GetCheckByAccountNumber ищет счет по его публичному номеру (account_number)
func GetCheckByAccountNumber(accountNumber string) (*structs.Check, error) {
	var check structs.Check

	err := engine.DB.Where("account_number = ?", accountNumber).First(&check).Error
	if err != nil {
		return nil, err
	}

	return &check, nil
}