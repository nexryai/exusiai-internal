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

func ConfigureRoutes(mux *http.ServeMux, queueController *controller.QueueController, objectDir string) error {
	configureHandler(mux, "GET /", controller.HandleHeartbeat)
	configureHandler(mux, "POST /v1/queue/add", queueController.HandleAddQueue)
	configureHandler(mux, "GET /v1/queue/{id}/status", queueController.HandleQueueStatus)

	if objectDir != "" {
		log.Printf("Route added: GET /objects/")
		mux.Handle("GET /objects/", http.StripPrefix("/objects/", http.FileServer(http.Dir(objectDir))))
	}

	return nil
}
