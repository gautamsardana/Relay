package main

import (
	"log"
	"fmt"

	"github.com/gautamsardana/relay/internal/config"
)

func main() {
	config, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}
	fmt.Println(config.Env.DatabaseURL)
}