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
	"github.com/gautamsardana/relay/internal/tools"
)

type Worker struct {
    store    *store.Store
	queue    *amqp.Connection
	registry *tools.Registry
	count 	 int
}

func New(conf *config.Config, s *store.Store, conn *amqp.Connection, r *tools.Registry) *Worker {
	worker := &Worker {
		store: s,
		queue: conn,
		registry: r,
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
	// check if already picked up by a different worker

	slog.Info("consumed a new request", "workflowID: ", event.WorkflowID)
    ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Minute)
    defer cancel()

	slog.Info("fetching step from store", "workflow_id", event.WorkflowID, "step_id", event.StepID)
    step, err := w.store.GetStepByID(ctx, event.StepID)
    if err != nil {
        return fmt.Errorf("failed to fetch step: %w", err)
    }

    slog.Info("updating step as processing", "workflow_id", step.WorkflowID, "step_id", step.StepID)
    err = w.store.UpdateStepStatus(ctx, step.StepID, models.StepStatusProcessing, "")
    if err != nil {
        return fmt.Errorf("failed to update step status: %w", err)
    }

    slog.Info("executing", "workflow_id", step.WorkflowID, "step_id", step.StepID, "tool", step.Tool)
	tool, exists := w.registry.Get(step.Tool)
	if !exists {
		w.failStep(ctx, &step, fmt.Errorf("tool does not exist in the registry, tool: %s", step.Tool))
		return fmt.Errorf("tool does not exist in the registry, tool: %s", step.Tool)
	}
	
	result, err := tool.Execute(ctx, step.Input)
	if err != nil {
		w.failStep(ctx, &step, err)
		return err
	}

	fmt.Println(result)
	
    // 4. mark step complete, publish next step
	// 4.1 add CREATE INDEX idx_steps_workflow_step ON steps(workflow_id, step_number);
	// 4.2 add a new query to find next step
    return nil
}

func (w *Worker) failStep(ctx context.Context, step *models.Step, err error) {
    w.store.UpdateStepStatus(ctx, step.WorkflowID, models.StepStatusFailed, err.Error())
    slog.Error("step failed", "workflow_id", step.WorkflowID, "step_id", step.WorkflowID, "error", err)
}