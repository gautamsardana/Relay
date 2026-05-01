package main

import(
	"log"
	"/internal/config"
)

func main() {
	config, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}
	print(config)
}