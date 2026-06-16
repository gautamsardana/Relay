package planner

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/queue"
	"github.com/google/uuid"
)

func (p *Planner) HandleRun(worker models.Worker, run models.Run) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	slog.Info("generating plan", "run_id", run.RunID, "worker_id", worker.WorkerID)
	steps, err := p.agent.GeneratePlan(ctx, worker.Instructions, p.registry.All())
	if err != nil {
		p.failRun(ctx, &run, err)
		return
	}

	slog.Info("validating tools", "run_id", run.RunID)
	for _, step := range steps {
		_, toolExists := p.registry.Get(step.Tool)
		if !toolExists {
			p.failRun(ctx, &run, fmt.Errorf("invalid tool used in steps: %s", step.Tool))
			return
		}
	}

	slog.Info("writing steps to store", "run_id", run.RunID)
	modelSteps := make([]models.Step, len(steps))

	for i, step := range steps {
		stepID, _ := uuid.NewV7()

		modelSteps[i] = models.Step{
			StepID:      stepID.String(),
			RunID:       run.RunID,
			StepNumber:  step.StepNumber,
			Tool:        step.Tool,
			Description: step.Description,
			Input:       step.Input,
			Status:      models.StepStatusInit,
			RetryCount:  0,
		}
	}

	err = p.store.InsertSteps(ctx, modelSteps)
	if err != nil {
		p.failRun(ctx, &run, err)
		return
	}

	slog.Info("marking run as processing", "run_id", run.RunID)
	err = p.store.UpdateRunStatus(ctx, run.RunID, models.RunStatusProcessing, "")
	if err != nil {
		p.failRun(ctx, &run, err)
		return
	}

	slog.Info("publishing first step", "run_id", run.RunID)
	sort.Slice(modelSteps, func(i, j int) bool {
		return modelSteps[i].StepNumber < modelSteps[j].StepNumber
	})
	if len(modelSteps) == 0 {
		p.failRun(ctx, &run, fmt.Errorf("no steps defined for this run"))
		return
	}
	firstStep := modelSteps[0]

	err = p.store.MarkStepPending(ctx, firstStep.StepID)
	if err != nil {
		p.failRun(ctx, &run, fmt.Errorf("failed to mark first step pending: %w", err))
		return
	}

	err = p.queue.PublishStep(ctx, queue.StepEvent{RunID: run.RunID, StepID: firstStep.StepID})
	if err != nil {
		p.failRun(ctx, &run, err)
		return
	}
}

func (p *Planner) failRun(ctx context.Context, run *models.Run, err error) {
	p.store.UpdateRunStatus(ctx, run.RunID, models.RunStatusFailed, err.Error())
	slog.Error("run failed", "run_id", run.RunID, "error", err)
}
