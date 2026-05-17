package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/gautamsardana/relay/internal/config"
	"github.com/gautamsardana/relay/internal/queue"
	"github.com/gautamsardana/relay/internal/store"
	"github.com/gautamsardana/relay/internal/tools"
	"github.com/gautamsardana/relay/internal/worker"
)

func main(){
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)
	
	config, err := config.LoadConfig()
	failOnError(err, "Failed to load config")
	
	store, err := store.New(config)
	failOnError(err, "Failed to connect to DB")
	defer store.Conn.Close()
	
	conn, err := queue.Dial(config)
	failOnError(err, "Failed to connect to queue")
	defer conn.Close()

	registry := tools.NewRegistry()
	registry.Register(tools.NewWebSearch(config))

	worker := worker.New(config, store, conn, registry)
	worker.SpawnWorkers()

	select{}
}

func failOnError(err error, msg string) {
  if err != nil {
    log.Fatalf("%s: %s", msg, err)
  }
}