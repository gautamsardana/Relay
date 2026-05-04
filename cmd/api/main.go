package main

import (
	"log"

	"github.com/gautamsardana/relay/internal/config"
	"github.com/gautamsardana/relay/internal/queue"
)

func main() {
	config, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}
	
	queue.Connect(config)
	defer config.Queue.Close()
}