package mysql

import (
	"fmt"
	"log"
	"os"
	"strings"

	"florenbot/backend/engine/structs"
	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// DB — глобальная переменная для работы с базой данных
var DB *gorm.DB

func isDBDebug() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG")))
	return v == "1" || v == "true" || v == "yes"
}

// Debug выводит отладочное сообщение в консоль
func Debug(message string, args ...interface{}) {
	fmt.Printf("[DEBUG] "+message+"\n", args...)
}

// ConnectDB устанавливает соединение с базой данных и выполняет миграции
func ConnectDB() {
	dbg := isDBDebug()

	// 1. Загружаем переменные окружения из .env, игнорируем ошибку, если файла нет (для Docker)
	if err := godotenv.Load(); err != nil {
		log.Println("Внимание: .env файл не найден (используем системные переменные окружения)")
		if dbg {
			log.Printf("[DEBUG][DB] godotenv.Load() err: %v", err)
		}
	} else if dbg {
		log.Printf("[DEBUG][DB] .env загружен")
	}

	dbUser := os.Getenv("DB_USER")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")

	if dbg {
		log.Printf("[DEBUG][DB] Параметры подключения: user=%q, host=%q, port=%q, name=%q, password_set=%v",
			dbUser, dbHost, dbPort, dbName, os.Getenv("DB_PASSWORD") != "")
	}

	// 2. Формируем строку подключения (DSN) для MySQL
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		dbUser,
		os.Getenv("DB_PASSWORD"),
		dbHost,
		dbPort,
		dbName,
	)

	// 3. Подключаемся и записываем результат в глобальную переменную DB
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: false,
		TranslateError:                         true,
	})
	if err != nil {
		if dbg {
			log.Printf("[DEBUG][DB] Ошибка gorm.Open для %s@%s:%s/%s: %v", dbUser, dbHost, dbPort, dbName, err)
		}
		log.Panicf("Не удалось подключиться к базе данных: %v", err)
	}

	if dbg {
		log.Printf("[DEBUG][DB] Соединение GORM установлено (DB != nil: %v)", DB != nil)
	}

	// 4. Автоматически запускаем миграции таблиц после успешного подключения
	if dbg {
		log.Printf("[DEBUG][DB] Запуск Migrate(DB)...")
	}
	if err := Migrate(DB); err != nil {
		if dbg {
			log.Printf("[DEBUG][DB] Ошибка Migrate(DB): %v", err)
		}
		log.Panicf("Ошибка при выполнении миграций: %v", err)
	}

	if dbg {
		log.Printf("[DEBUG][DB] Миграции завершены без ошибок")
	}
	fmt.Println("Подключение к БД и миграции выполнены успешно!")
}

// Migrate выполняет автоматическую миграцию таблиц и инициализацию данных
func Migrate(db *gorm.DB) error {
	dbg := isDBDebug()

	// 1. Создаем независимые таблицы (фундамент)
	if dbg {
		log.Printf("[DEBUG][DB][MIGRATE] Шаг 1: Chat, Clan")
	}
	err := db.AutoMigrate(
		&structs.Chat{},
		&structs.Clan{},
	)
	if err != nil {
		return err
	}

	// 2. Создаем таблицу ролей
	if dbg {
		log.Printf("[DEBUG][DB][MIGRATE] Шаг 2: Role")
	}
	err = db.AutoMigrate(&structs.Role{})
	if err != nil {
		return err
	}

	// 3. Создаем таблицы, которые зависят от ролей или чатов
	if dbg {
		log.Printf("[DEBUG][DB][MIGRATE] Шаг 3: Member, User")
	}
	err = db.AutoMigrate(
		&structs.Member{},
		&structs.User{},
	)
	if err != nil {
		return err
	}

	// 4. Создаем все остальные зависимые таблицы
	if dbg {
		log.Printf("[DEBUG][DB][MIGRATE] Шаг 4: Check, Role, UserCommandRestriction, UserPromo, Report, Ban, ClanMember, ClanBlacklist")
	}
	return db.AutoMigrate(
		&structs.Check{},
		&structs.Role{},
		&structs.UserCommandRestriction{},
		&structs.UserPromo{},
		&structs.Report{},
		&structs.Ban{},
		&structs.ClanMember{},
		&structs.ClanBlacklist{},
	)
}

// GetUserBalance получает баланс пользователя по его ID
func GetUserBalance(userId uint64) (float64, error) {
	if isDBDebug() {
		log.Printf("[DEBUG][DB] GetUserBalance: user_id=%d, DB_is_nil=%v", userId, DB == nil)
	}
	var balance float64
	result := DB.Model(&structs.User{}).Where("id = ?", userId).Pluck("balance", &balance)
	if result.Error != nil {
		if isDBDebug() {
			log.Printf("[DEBUG][DB] GetUserBalance ошибка для user_id=%d: %v", userId, result.Error)
		}
		return 0, result.Error
	}
	// Если записей нет, возвращаем ошибку
	if result.RowsAffected == 0 {
		return 0, fmt.Errorf("пользователь с ID %d не найден", userId)
	}
	return balance, nil
}

// UpdateBalance изменяет баланс пользователя (добавляет или вычитает сумму)
func UpdateBalance(id uint64, amount float64) error {
	if isDBDebug() {
		log.Printf("[DEBUG][DB] UpdateBalance: id=%d, amount=%.2f, DB_is_nil=%v", id, amount, DB == nil)
	}
	result := DB.Model(&structs.User{}).Where("id = ?", id).Update("balance", gorm.Expr("balance + ?", amount))
	if result.Error != nil && isDBDebug() {
		log.Printf("[DEBUG][DB] UpdateBalance ошибка для id=%d: %v", id, result.Error)
	}
	return result.Error
}

// CloseDB корректно закрывает соединение с базой данных
func CloseDB() {
	if DB == nil {
		if isDBDebug() {
			log.Printf("[DEBUG][DB] CloseDB: DB уже nil")
		}
		return
	}
	if isDBDebug() {
		log.Printf("[DEBUG][DB] CloseDB: закрытие соединения с БД...")
	}
	sqlDB, err := DB.DB()
	if err != nil {
		log.Printf("Не удалось получить экземпляр БД для закрытия: %v", err)
		return
	}
	if err := sqlDB.Close(); err != nil {
		log.Printf("Ошибка при закрытии соединения с БД: %v", err)
	} else if isDBDebug() {
		log.Printf("[DEBUG][DB] Соединение с БД закрыто")
	}
}