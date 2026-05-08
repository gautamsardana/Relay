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

func (q *QueueManager) PublishStep(config *config.Config, event StepEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	body := "Hello World!"
	err := q.Channel.PublishWithContext(ctx,
	"",     // exchange
	q.Queue.Name, // routing key
	false,  // mandatory
	false,  // immediate
	amqp.Publishing {
		ContentType: "text/plain",
		Body:        []byte(body),
	})

	if err != nil {
		return err
	}
	log.Printf(" [x] Sent %s\n", body)

	return nil
}

func (q *QueueManager) ConsumeSteps(ch *amqp.Channel, handler func(StepEvent) error) error {
	return nil
}