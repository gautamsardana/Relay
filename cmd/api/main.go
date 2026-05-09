package main

import (
	"fmt"
	"log"

	"github.com/gautamsardana/relay/internal/agent"
	"github.com/gautamsardana/relay/internal/api"
	"github.com/gautamsardana/relay/internal/config"
	"github.com/gautamsardana/relay/internal/queue"
	"github.com/gautamsardana/relay/internal/store"
)

func main() {
	config, err := config.LoadConfig()
	failOnError(err, "Failed to load config")
	fmt.Println(config.App.AIPrimary, config.App.AISecondary)
	
	store, err := store.New(config)
	failOnError(err, "Failed to connect to DB")
	defer store.Conn.Close()
	
	q, err := queue.New(config)
	failOnError(err, "Failed to connect to queue")
	defer q.Conn.Close()
	defer q.Channel.Close()

	// 3. initialize agents
	_, err = agent.NewAgentManager(config)
	failOnError(err, "Failed to connect to agents")

	// 4. initialize tools

	// 5. initialize planner
	api.ListenAndServe()
}

func failOnError(err error, msg string) {
  if err != nil {
    log.Fatalf("%s: %s", msg, err)
  }
}

/*
func main() {
    cfg := config.Load()

    db := store.NewPostgresStore(cfg.DBURL)

    claudeAgent := agent.NewClaudeAgent(cfg.ClaudeAPIKey)

    rabbit := queue.NewPublisher(cfg.RabbitURL)

    planner := planner.NewPlanner(
        db,
        claudeAgent,
        rabbit,
    )

    server := api.NewServer(planner)

    server.Start()
}
*/