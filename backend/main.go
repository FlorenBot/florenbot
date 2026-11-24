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

	"florenbot/backend/routes"
	"florenbot/services"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("env not found, using system environment variables")
	}

	apiUrl := os.Getenv("API_URL")
	if apiUrl == "" {
		log.Panic("Укажите API_URL")
	}

	httpSvc := services.NewHTTPService(apiUrl, 0)

	mux := http.NewServeMux()
	routes.RegisterRoutes(mux, httpSvc)

	addr := ":8080"
	log.Printf("REST API listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}