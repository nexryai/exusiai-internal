package router

import (
	"log"
	"net/http"

	"github.com/nexryai/exusiai-internal/internal/controller"
)

func configureHandler(mux *http.ServeMux, path string, handler func(http.ResponseWriter, *http.Request)) {
	log.Printf("Route added: %s", path)
	mux.HandleFunc(path, handler)
}

func ConfigureRoutes(mux *http.ServeMux) error {
	configureHandler(mux, "GET /", controller.HandleHeartbeat)

	return nil
}