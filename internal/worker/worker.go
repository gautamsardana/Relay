package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/gautamsardana/relay/internal/config"
	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/queue"
	"github.com/gautamsardana/relay/internal/store"
)

type Worker struct {
    store    *store.Store
	queue    *amqp.Connection
	count 	 int
}

func New(conf *config.Config, s *store.Store, conn *amqp.Connection) *Worker {
	worker := &Worker {
		store: s,
		queue: conn,
		count: conf.App.WorkerCount,
	}
	return worker
}

func (w *Worker) SpawnWorkers() {
    for i := range w.count {
		qm, err := queue.New(w.queue)
		if err != nil {
			slog.Error("failed to create worker queue manager", "worker_id", i, "error", err)
			continue
    	}
		
        go func(id int, qm *queue.QueueManager) {
            slog.Info("worker started", "worker_id", id)
            if err := qm.ConsumeSteps(i, w.HandleStep); err != nil {
                slog.Error("worker stopped", "worker_id", id, "error", err)
            }
        }(i, qm)
    }
}

func (w *Worker) HandleStep(event queue.StepEvent) error {
    ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Minute)
    defer cancel()

    slog.Info("handling step", "workflow_id", event.WorkflowID, "step_id", event.StepID)

    // 1. fetch step from DB
    step, err := w.store.GetStepByID(ctx, event.StepID)
    if err != nil {
        return fmt.Errorf("failed to fetch step: %w", err)
    }

    // 2. mark step as running
    err = w.store.UpdateStepStatus(ctx, step.StepID, models.StepStatusProcessing)
    if err != nil {
        return fmt.Errorf("failed to update step status: %w", err)
    }

    // 3. execute tool (coming later)

    // 4. mark step complete, publish next step
	// 4.1 add CREATE INDEX idx_steps_workflow_step ON steps(workflow_id, step_number);
	// 4.2 add a new query to find next step
    return nil
}