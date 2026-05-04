package queue

import (
	"log"

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
	config.Queue = conn
}

func PublishStep(ch *amqp.Channel, event StepEvent) error{
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