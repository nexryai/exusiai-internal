package server

import (
	"fmt"
	"log"
	"net/http"

	"github.com/nexryai/exusiai-internal/internal/controller"
	"github.com/nexryai/exusiai-internal/internal/router"
	"github.com/pkg/errors"
)

var (
	ErrFailedToConfigureRoutes = errors.New("failed to configure routes")
	ErrFailedToStartServer     = errors.New("failed to start server")
)

func New(port string, queueController *controller.QueueController, objectDir string) (*http.Server, error) {
	log.Printf("Initializing server... (port: %s)", port)
	mux := http.NewServeMux()

	err := router.ConfigureRoutes(mux, queueController, objectDir)
	if err != nil {
		return nil, errors.Wrap(err, ErrFailedToConfigureRoutes.Error())
	}

	return &http.Server{
		Addr:    fmt.Sprintf(":%s", port),
		Handler: mux,
	}, nil
}

func InitializeServer(port string, queueController *controller.QueueController, objectDir string) error {
	srv, err := New(port, queueController, objectDir)
	if err != nil {
		return err
	}

	err = srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return errors.Wrap(err, ErrFailedToStartServer.Error())
	}

	return nil
}
