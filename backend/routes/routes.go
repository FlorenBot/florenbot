package routes

import (
	"fmt"
	"net/http"

	_ "florenbot/backend/docs"
	"florenbot/backend/routes/handlers"
	"florenbot/services"

	httpSwagger "github.com/swaggo/http-swagger/v2"
)

func RegisterRoutes(mux *http.ServeMux, httpSvc *services.HTTPService) {
	mux.Handle("/docs/", http.StripPrefix("/docs", httpSwagger.WrapHandler))

	fmt.Println("Swagger запущен на http://localhost:8080/docs")

	standard := handlers.NewStandardHandler(httpSvc)
	standard.Register(mux)
}