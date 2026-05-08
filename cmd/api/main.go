package main

import (
	"log"

	"github.com/gautamsardana/relay/internal/config"
	"github.com/gautamsardana/relay/internal/queue"
	"github.com/gautamsardana/relay/internal/store"
)

func main() {
	config, err := config.LoadConfig()
	failOnError(err, "Failed to load config")
	
	store, err := store.New(config)
	failOnError(err, "Failed to connect to DB")
	defer store.Conn.Close()
	
	q, err := queue.New(config)
	failOnError(err, "Failed to connect to queue")
	defer q.Conn.Close()
	defer q.Channel.Close()

	// 3. initialize agent
	// 4. initialize tools
	// 5. start server 
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