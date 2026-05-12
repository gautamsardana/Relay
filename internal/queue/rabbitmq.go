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

func (q *QueueManager) ConsumeSteps(handler func(StepEvent) error) error {
	// ch, err := q.Conn.Channel()
    // if err != nil {
    //     return fmt.Errorf("failed to create channel: %w", err)
    // }
    // defer ch.Close()

    msgs, err := q.Channel.Consume(
        q.Queue.Name,
        "",    // consumer tag
        false, // auto-ack — important: we manually ack after processing
        false, // exclusive
        false, // no-local
        false, // no-wait
        nil,
    )
    if err != nil {
        return fmt.Errorf("failed to start consuming: %w", err)
    }

    for msg := range msgs {
        var event StepEvent
        if err := json.Unmarshal(msg.Body, &event); err != nil {
            slog.Error("failed to unmarshal step event", "error", err)
            msg.Nack(false, false) // discard malformed message
            continue
        }

        if err := handler(event); err != nil {
            slog.Error("failed to handle step event", "error", err, "step_id", event.StepID)
            msg.Nack(false, true) // requeue on failure
            continue
        }

        msg.Ack(false)
    }
    return nil
}