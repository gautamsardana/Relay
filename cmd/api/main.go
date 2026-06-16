package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/gautamsardana/relay/internal/agent"
	"github.com/gautamsardana/relay/internal/api"
	"github.com/gautamsardana/relay/internal/config"
	"github.com/gautamsardana/relay/internal/planner"
	"github.com/gautamsardana/relay/internal/queue"
	"github.com/gautamsardana/relay/internal/store"
	"github.com/gautamsardana/relay/internal/tools"
)

func main() {
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

	plannerQueue, err := queue.New(conn)
	failOnError(err, "Failed to create planner queue")

	agent, err := agent.NewAgentManager(config)
	failOnError(err, "Failed to connect to agents")

	registry := tools.NewRegistry()
	registry.Register(tools.NewWebSearch(config))
	registry.Register(tools.NewHTTPRequest())
	registry.Register(tools.NewDocumentRead())

	planner := planner.New(config, store, plannerQueue, agent, registry)
	planner.StartReconciler()
	planner.StartScheduler()

	server := api.New(planner)
	server.ListenAndServe()
}

func failOnError(err error, msg string) {
  if err != nil {
    log.Fatalf("%s: %s", msg, err)
  }
}