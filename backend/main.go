package backend

import (
	"log"
	"net/http"
	"os"

	"florenbot/backend/routes"
	"florenbot/services"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("env not found, using system environment variables")
	}

	apiUrl := os.Getenv("API_URL")


	if apiUrl == "" {
		log.Panic("Укажите API_URL")
		panic("Укажите API_URL")
	}

	httpSvc := services.NewHTTPService(apiUrl, 0)

	mux := http.NewServeMux()
	standard := routes.NewStandardHandler(httpSvc)
	standard.Register(mux)

	addr := ":8080"
	log.Printf("REST API listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}