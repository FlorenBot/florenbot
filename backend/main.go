// @title FlorenBot REST API
// @version 1.0
// @description REST API бэкенда для FlorenBot
// @host localhost:8080
// @BasePath /
package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"florenbot/backend/engine/cache"
	"florenbot/backend/engine/mysql"
	"florenbot/backend/routes"
	"florenbot/services"

	"github.com/joho/godotenv"
)

func isMainDebug() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG")))
	return v == "1" || v == "true" || v == "yes"
}

func main() {
	dbg := isMainDebug()

	if dbg {
		if wd, err := os.Getwd(); err == nil {
			log.Printf("[DEBUG][MAIN] Рабочая директория: %s", wd)
		}
	}

	if err := godotenv.Load(); err != nil {
		log.Println("env not found, using system environment variables")
		if dbg {
			log.Printf("[DEBUG][MAIN] godotenv.Load() err: %v", err)
		}
	} else if dbg {
		log.Printf("[DEBUG][MAIN] .env успешно загружен")
	}

	apiUrl := strings.TrimSpace(os.Getenv("API_URL"))
	if apiUrl == "" {
		log.Panic("Укажите API_URL")
	}

	appKey := strings.TrimSpace(os.Getenv("APP_KEY"))
	if appKey == "" {
		log.Panic("Укажите APP_KEY")
	}

	if dbg {
		log.Printf("[DEBUG][MAIN] Переменные окружения проверены: API_URL=%s, APP_KEY_SET=true (len=%d), CACHE_ENGINE=%q, REDIS_ADDR=%q, DB_HOST=%q, DB_PORT=%q, DB_NAME=%q",
			apiUrl, len(appKey), os.Getenv("CACHE_ENGINE"), os.Getenv("REDIS_ADDR"),
			os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_NAME"))
	}

	cache.InitCache()
	defer cache.ShutdownCache()

	if dbg {
		log.Printf("[DEBUG][MAIN] После InitCache: RDB_is_nil=%v", cache.GetRedis() == nil)
	}

	if dbg {
		log.Printf("[DEBUG][MAIN] Подключение к БД через mysql.ConnectDB()...")
	}
	mysql.ConnectDB()
	defer mysql.CloseDB()

	if dbg {
		log.Printf("[DEBUG][MAIN] После ConnectDB: DB_is_nil=%v", mysql.DB == nil)
	}

	httpSvc := services.NewHTTPService(apiUrl, 0)

	mux := http.NewServeMux()
	routes.RegisterRoutes(mux, httpSvc)

	addr := ":8080"
	port := 8080
	if dbg {
		log.Printf("[DEBUG][MAIN] Регистрация маршрутов завершена, запуск сервера на %s", addr)
	}
	log.Printf("Запущен на http://localhost:%d", port)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}