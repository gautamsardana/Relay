package queue

import (
	"context"
	"log"
	"time"

	"github.com/gautamsardana/relay/internal/config"
	amqp "github.com/rabbitmq/amqp091-go"
)

type StepEvent struct {
    WorkflowID string `json:"workflow_id"`
    StepID     string `json:"step_id"`
}

func Connect(config *config.Config) {
	conn, err := amqp.Dial(config.Env.QueueURL)
	failOnError(err, "Failed to connect to RabbitMQ")
	config.QueueConfig.Conn = conn

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	config.QueueConfig.Channel = ch

	q, err := ch.QueueDeclare(
		"steps1", // name
		true,    // durability
		false,   // delete when unused
		false,   // exclusive
		false,   // no-wait
		amqp.Table{
			amqp.QueueTypeArg: amqp.QueueTypeQuorum,
		},
	)
	failOnError(err, "Failed to declare a queue")
	config.QueueConfig.Queue = q

}

func PublishStep(config *config.Config, event StepEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	body := "Hello World!"
	err := config.QueueConfig.Channel.PublishWithContext(ctx,
	"",     // exchange
	config.QueueConfig.Queue.Name, // routing key
	false,  // mandatory
	false,  // immediate
	amqp.Publishing {
		ContentType: "text/plain",
		Body:        []byte(body),
	})
	failOnError(err, "Failed to publish a message")
	log.Printf(" [x] Sent %s\n", body)

	return nil
}

func ConsumeSteps(ch *amqp.Channel, handler func(StepEvent) error) error {
	return nil
}

func failOnError(err error, msg string) {
  if err != nil {
    log.Panicf("%s: %s", msg, err)
  }
}