package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gautamsardana/relay/internal/config"
	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/queue"
	"github.com/gautamsardana/relay/internal/store"
)

type Worker struct {
    store    *store.Store
	queue    *queue.QueueManager
	count 	 int
}

func New(conf *config.Config, s *store.Store, q *queue.QueueManager) *Worker {
	return &Worker {
		store: s,
		queue: q,
		count: conf.App.WorkerCount,
	}
}

func (w *Worker) SpawnWorkers() {
    for i := range w.count {
        go func(id int) {
            slog.Info("worker started", "worker_id", id)
            if err := w.queue.ConsumeSteps(w.HandleStep); err != nil {
                slog.Error("worker stopped", "worker_id", id, "error", err)
            }
        }(i)
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

	slog.Info("handling step", "workflow_id", event.WorkflowID, "step_id", event.StepID, "reached------")

    // 3. execute tool (coming later)

    // 4. mark step complete, publish next step
	// 4.1 add CREATE INDEX idx_steps_workflow_step ON steps(workflow_id, step_number);
	// 4.2 add a new query to find next step
    return nil
}