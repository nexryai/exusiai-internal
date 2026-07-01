package server

import (
	"fmt"
	"log"
	"net/http"

	"github.com/nexryai/exusiai-internal/internal/router"
	"github.com/pkg/errors"
)

var (
	ErrFailedToConfigureRoutes = errors.New("failed to configure routes")
	ErrFailedToStartServer     = errors.New("failed to start server")
)

func InitializeServer(port string) error {
	log.Printf("Initializing server... (port: %s)", port)
	mux := http.NewServeMux()

	err := router.ConfigureRoutes(mux)
	if err != nil {
		return errors.Wrap(err, ErrFailedToConfigureRoutes.Error())
	}

	err = http.ListenAndServe(fmt.Sprintf(":%s", port), mux)
	if err != nil {
		return errors.Wrap(err, ErrFailedToStartServer.Error())
	}

	return nil
}