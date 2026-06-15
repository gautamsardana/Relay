package worker

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/gautamsardana/relay/internal/config"
	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/queue"
	"github.com/gautamsardana/relay/internal/store"
	"github.com/gautamsardana/relay/internal/tools"
)

type Worker struct {
	store      *store.Store
	queue      *amqp.Connection
	registry   *tools.Registry
	count      int
	maxRetries int
}

func New(conf *config.Config, s *store.Store, conn *amqp.Connection, r *tools.Registry) *Worker {
	return &Worker{
		store:      s,
		queue:      conn,
		registry:   r,
		count:      conf.App.WorkerCount,
		maxRetries: conf.App.MaxStepRetries,
	}
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
	slog.Info("consumed a new request", "workflowID", event.WorkflowID, "stepID", event.StepID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	slog.Info("claiming step", "workflow_id", event.WorkflowID, "step_id", event.StepID)
	step, claimed, err := w.store.ClaimStep(ctx, event.StepID)
	if err != nil {
		return fmt.Errorf("failed to claim step: %w", err)
	}
	if !claimed {
		slog.Info("step already claimed by another worker, skipping", "step_id", event.StepID)
		return nil
	}

	slog.Info("validating tool", "workflow_id", step.WorkflowID, "step_id", step.StepID, "tool", step.Tool)
	fmt.Println(w.registry.Names())
	tool, exists := w.registry.Get(step.Tool)
	if !exists {
		w.failStep(ctx, &step, fmt.Errorf("tool does not exist in the registry, tool: %s", step.Tool))
		return fmt.Errorf("tool does not exist in the registry, tool: %s", step.Tool)
	}

	slog.Info("resolving step inputs", "workflow_id", step.WorkflowID, "step_id", step.StepID)
	resolvedInput, err := w.resolveInputs(ctx, step.WorkflowID, step.Input)
	if err != nil {
		w.failStep(ctx, &step, fmt.Errorf("failed to resolve inputs: %w", err))
		return err
	}

	slog.Info("executing step", "workflow_id", step.WorkflowID, "step_id", step.StepID, "tool", step.Tool)
	result, err := tool.Execute(ctx, resolvedInput)
	if err != nil {
		return w.handleStepError(ctx, qm, &step, err)
	}

	slog.Info("updating step as completed", "workflow_id", step.WorkflowID, "step_id", step.StepID)
	err = w.store.UpdateStepAsCompleted(ctx, step.StepID, result)
	if err != nil {
		return fmt.Errorf("failed to update step status: %w", err)
	}

	slog.Info("looking for next step", "workflow_id", step.WorkflowID, "step_id", step.StepID)
	nextStep, hasNext, err := w.store.GetStepByWorkflowAndNumber(ctx, step.WorkflowID, step.StepNumber+1)
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

	if err = w.store.MarkStepPending(ctx, nextStep.StepID); err != nil {
		return fmt.Errorf("failed to mark next step pending: %w", err)
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

// resolveInputs replaces {{steps[N].output.FIELD}} templates in step inputs
// with actual output values from prior steps fetched from Postgres.
var templateRegex = regexp.MustCompile(`\{\{steps\[(\d+)\]\.output\.([^}]+)\}\}`)

func (w *Worker) resolveInputs(ctx context.Context, workflowID string, input map[string]any) (map[string]any, error) {
	resolved := make(map[string]any, len(input))
	for key, val := range input {
		strVal, ok := val.(string)
		if !ok {
			resolved[key] = val
			continue
		}

		result, err := w.resolveString(ctx, workflowID, strVal)
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", key, err)
		}
		resolved[key] = result
	}
	return resolved, nil
}

func (w *Worker) resolveString(ctx context.Context, workflowID string, s string) (any, error) {
	matches := templateRegex.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		return s, nil
	}

	// If the entire string is a single template, return the typed value directly
	// (e.g. the output field might be a list, not a string)
	if len(matches) == 1 {
		loc := matches[0]
		if loc[0] == 0 && loc[1] == len(s) {
			stepNum, _ := strconv.Atoi(s[loc[2]:loc[3]])
			field := s[loc[4]:loc[5]]
			return w.lookupOutput(ctx, workflowID, stepNum, field)
		}
	}

	// Multiple templates or templates embedded in a larger string — resolve to string
	result := templateRegex.ReplaceAllStringFunc(s, func(match string) string {
		parts := templateRegex.FindStringSubmatch(match)
		stepNum, _ := strconv.Atoi(parts[1])
		field := parts[2]
		val, err := w.lookupOutput(ctx, workflowID, stepNum, field)
		if err != nil {
			slog.Warn("template resolution failed", "match", match, "error", err)
			return match // leave unresolved rather than silently emptying
		}
		return fmt.Sprintf("%v", val)
	})
	return result, nil
}

func (w *Worker) lookupOutput(ctx context.Context, workflowID string, stepNumber int, field string) (any, error) {
	step, found, err := w.store.GetStepByWorkflowAndNumber(ctx, workflowID, stepNumber)
	if err != nil {
		return nil, fmt.Errorf("step %d lookup failed: %w", stepNumber, err)
	}
	if !found {
		return nil, fmt.Errorf("step %d not found", stepNumber)
	}
	if step.Output == nil {
		return nil, fmt.Errorf("step %d has no output", stepNumber)
	}

	// Support nested field access: "results.items" → output["results"]["items"]
	parts := strings.Split(field, ".")
	var current any = step.Output
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("cannot traverse field %q: not an object", part)
		}
		current, ok = m[part]
		if !ok {
			return nil, fmt.Errorf("field %q not found in step %d output", part, stepNumber)
		}
	}
	return current, nil
}

func (w *Worker) handleStepError(ctx context.Context, qm *queue.QueueManager, step *models.Step, toolErr error) error {
	slog.Warn("step execution failed", "workflow_id", step.WorkflowID, "step_id", step.StepID, "error", toolErr, "retry_count", step.RetryCount)

	if err := w.store.IncrementStepRetryCount(ctx, step.StepID); err != nil {
		slog.Error("failed to increment retry count", "step_id", step.StepID, "error", err)
	}

	if step.RetryCount+1 < w.maxRetries {
		slog.Info("retrying step", "step_id", step.StepID, "attempt", step.RetryCount+1, "max", w.maxRetries)
		if err := w.store.UpdateStepStatus(ctx, step.StepID, models.StepStatusPending, ""); err != nil {
			return fmt.Errorf("failed to reset step to pending: %w", err)
		}
		if err := qm.PublishStep(ctx, queue.StepEvent{WorkflowID: step.WorkflowID, StepID: step.StepID}); err != nil {
			return fmt.Errorf("failed to re-publish step for retry: %w", err)
		}
		return nil
	}

	slog.Error("step exhausted retries", "step_id", step.StepID, "max_retries", w.maxRetries)
	w.failStep(ctx, step, toolErr)
	return toolErr
}

func (w *Worker) failStep(ctx context.Context, step *models.Step, err error) {
	if updateErr := w.store.UpdateStepStatus(ctx, step.StepID, models.StepStatusFailed, err.Error()); updateErr != nil {
		slog.Error("failed to mark step as failed", "workflow_id", step.WorkflowID, "step_id", step.StepID, "error", updateErr)
	}
	if updateErr := w.store.UpdateWorkflowStatus(ctx, step.WorkflowID, models.WorkflowStatusFailed, err.Error()); updateErr != nil {
		slog.Error("failed to mark workflow as failed", "workflow_id", step.WorkflowID, "error", updateErr)
	}
	slog.Error("step failed", "workflow_id", step.WorkflowID, "step_id", step.StepID, "error", err)

	// Cancel remaining unstarted steps in the background — non-critical.
	// If this fails, those steps sit as init/pending harmlessly.
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		reason := "cancelled: step " + step.StepID + " failed"
		if updateErr := w.store.CancelUnstartedSteps(bgCtx, step.WorkflowID, reason); updateErr != nil {
			slog.Warn("failed to cancel unstarted steps (non-critical)", "workflow_id", step.WorkflowID, "error", updateErr)
		}
	}()
}
