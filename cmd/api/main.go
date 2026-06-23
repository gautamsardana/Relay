package main

import (
	"context"
	"log"
	"log/slog"
	"os"

	"github.com/gautamsardana/relay/internal/agent"
	"github.com/gautamsardana/relay/internal/api"
	"github.com/gautamsardana/relay/internal/catalog"
	"github.com/gautamsardana/relay/internal/config"
	"github.com/gautamsardana/relay/internal/discovery"
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

	if err := catalog.Seed(context.Background(), store); err != nil {
		slog.Error("failed to seed company catalog", "error", err)
	}

	conn, err := queue.Dial(config)
	failOnError(err, "Failed to connect to queue")
	defer conn.Close()

	plannerQueue, err := queue.New(conn)
	failOnError(err, "Failed to create planner queue")

	agent, err := agent.NewAgentManager(config)
	failOnError(err, "Failed to connect to agents")

	registry := tools.BuildRegistry(config, store, agent)

	planner := planner.New(config, store, plannerQueue, agent, registry)
	planner.StartReconciler()
	planner.StartScheduler()

	discoverer := discovery.New(store,
		discovery.NewDomainSearchSource(config.Env.TavilyApiKey),
		discovery.NewYCSource(),
	)
	discoverer.Start()

	server := api.New(planner)
	server.ListenAndServe()
}

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}
