package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/gautamsardana/relay/internal/config"
	amqp "github.com/rabbitmq/amqp091-go"
)

type StepEvent struct {
    WorkflowID string `json:"workflow_id"`
    StepID     string `json:"step_id"`
}

func New(config *config.Config) (*QueueManager, error){
	conn, err := amqp.Dial(config.Env.QueueURL)
	if err != nil {
		return nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}

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
	
	if err != nil {
		return nil, err
	}
	
	queueManager := &QueueManager{
		Conn: conn,
		Channel: ch,
		Queue: q,
	}
	return queueManager, nil
}

func (q *QueueManager) PublishStep(ctx context.Context, event StepEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal step event: %w", err)
	}

	err = q.Channel.PublishWithContext(ctx,
		"",           // exchange
		q.Queue.Name, // routing key
		false,        // mandatory
		false,        // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		})
	if err != nil {
		return fmt.Errorf("failed to publish step event: %w", err)
	}

	slog.Info("step event published", "workflow_id", event.WorkflowID, "step_id", event.StepID)
	return nil
}

func (q *QueueManager) ConsumeSteps(ctx context.Context, ch *amqp.Channel, handler func(StepEvent) error) error {
	return nil
}