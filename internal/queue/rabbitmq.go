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
    RunID  string `json:"run_id"`
    StepID string `json:"step_id"`
}

func Dial(config *config.Config) (*amqp.Connection, error) {
    return amqp.Dial(config.Env.QueueURL)
}

func New(conn *amqp.Connection) (*QueueManager, error) {
    ch, err := conn.Channel()
    if err != nil {
        return nil, err
    }

    q, err := ch.QueueDeclare(
        "steps1",
        true,
        false,
        false,
        false,
        amqp.Table{
            amqp.QueueTypeArg: amqp.QueueTypeQuorum,
        },
    )
    if err != nil {
        return nil, err
    }

    return &QueueManager{
        Conn:    conn,
        Channel: ch,
        Queue:   q,
    }, nil
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

	slog.Info("step event published", "run_id", event.RunID, "step_id", event.StepID)
	return nil
}

func (q *QueueManager) ConsumeSteps(handler func(StepEvent) error) error {
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
            msg.Nack(false, false) // requeue on failure
            continue
        }

        msg.Ack(false)
    }
    return nil
}