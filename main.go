package main

import (
	"log"

	"github.com/nexryai/exusiai-internal/internal/boot"
)

func main() {
	if err := boot.Run(); err != nil {
		log.Fatal(err)
	}
}
