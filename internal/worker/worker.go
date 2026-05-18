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
        go func(id int) {
            for {
                qm, err := queue.New(w.queue)
                if err != nil {
                    slog.Error("failed to create worker queue manager", "worker_id", id, "error", err)
                    time.Sleep(5 * time.Second)
                    continue
                }
                slog.Info("worker started", "worker_id", id)
                err = qm.ConsumeSteps(func(event queue.StepEvent) error {
                    return w.HandleStep(qm, event)
                })
                if err != nil {
                    slog.Error("worker stopped, restarting", "worker_id", id, "error", err)
                    time.Sleep(5 * time.Second)
                }
            }
        }(i)
    }
}

func (w *Worker) HandleStep(qm *queue.QueueManager, event queue.StepEvent) error {
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

	slog.Info("updating step as completed", "workflow_id", step.WorkflowID, "step_id", step.StepID)
    err = w.store.UpdateStepAsCompleted(ctx, step.StepID, result)
    if err != nil {
        return fmt.Errorf("failed to update step status: %w", err)
    }
	
	slog.Info("looking for next step", "workflow_id", step.WorkflowID, "step_id", step.StepID)
	nextStep, hasNext, err := w.store.GetNextStep(ctx, step.WorkflowID, int32(step.StepNumber)+1)
	if err != nil {
		slog.Error("failed to get next step", "workflow_id", step.WorkflowID, "error", err)
		return fmt.Errorf("failed to get next step: %w", err)
	}

	if !hasNext {
		// last step completed, mark workflow success
		err = w.store.UpdateWorkflowStatus(ctx, step.WorkflowID, models.WorkflowStatusSuccess, "")
		if err != nil {
			return fmt.Errorf("failed to mark workflow success: %w", err)
		}
		slog.Info("workflow completed", "workflow_id", step.WorkflowID)
		return nil
	}

	err = qm.PublishStep(ctx, queue.StepEvent{
		WorkflowID: nextStep.WorkflowID,
		StepID:     nextStep.StepID,
	})
	if err != nil {
		return fmt.Errorf("failed to publish next step: %w", err)
	}

	slog.Info("next step published", "workflow_id", nextStep.WorkflowID, "step_id", nextStep.StepID, "step_number", nextStep.StepNumber)
	return nil
	// add CREATE INDEX idx_steps_workflow_step ON steps(workflow_id, step_number);
}

func (w *Worker) failStep(ctx context.Context, step *models.Step, err error) {
    w.store.UpdateStepStatus(ctx, step.WorkflowID, models.StepStatusFailed, err.Error())
    slog.Error("step failed", "workflow_id", step.WorkflowID, "step_id", step.WorkflowID, "error", err)
}