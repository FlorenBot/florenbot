package routes

import (
	"net/http"

	_ "florenbot/backend/docs"
	"florenbot/backend/routes/handlers"
	"florenbot/services"

	httpSwagger "github.com/swaggo/http-swagger/v2"
)

func RegisterRoutes(mux *http.ServeMux, httpSvc *services.HTTPService) {
	mux.Handle("/swagger/", httpSwagger.WrapHandler)

	standard := handlers.NewStandardHandler(httpSvc)
	standard.Register(mux)
}