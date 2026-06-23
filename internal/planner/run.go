package planner

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/queue"
	"github.com/google/uuid"
)

// HandleRun builds the deterministic job-hunt pipeline (job_search → score_jobs)
// and publishes the first step. There is no LLM planning: the worker is always a
// job hunter, both steps read their config from the ExecutionContext, and the
// LLM is used only for scoring inside score_jobs. This removes the wrong-tool /
// template-resolution failures that LLM-generated plans caused.
func (p *Planner) HandleRun(worker models.Worker, run models.Run) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	slog.Info("building job-hunt plan", "run_id", run.RunID, "worker_id", worker.WorkerID)

	searchID, _ := uuid.NewV7()
	scoreID, _ := uuid.NewV7()
	modelSteps := []models.Step{
		{
			StepID:      searchID.String(),
			RunID:       run.RunID,
			StepNumber:  1,
			Tool:        "job_search",
			Description: "Find new matching jobs",
			Input:       map[string]any{},
			Status:      models.StepStatusInit,
		},
		{
			StepID:      scoreID.String(),
			RunID:       run.RunID,
			StepNumber:  2,
			Tool:        "score_jobs",
			Description: "Rank jobs by fit and recency",
			Input:       map[string]any{"jobs": "{{steps[1].output.jobs}}"},
			Status:      models.StepStatusInit,
		},
	}

	slog.Info("writing steps to store", "run_id", run.RunID)
	if err := p.store.InsertSteps(ctx, modelSteps); err != nil {
		p.failRun(ctx, &run, err)
		return
	}

	slog.Info("marking run as processing", "run_id", run.RunID)
	if err := p.store.UpdateRunStatus(ctx, run.RunID, models.RunStatusProcessing, ""); err != nil {
		p.failRun(ctx, &run, err)
		return
	}

	firstStep := modelSteps[0]
	slog.Info("marking step as pending", "step_id", firstStep.StepID)
	if err := p.store.MarkStepPending(ctx, firstStep.StepID); err != nil {
		p.failRun(ctx, &run, fmt.Errorf("failed to mark first step pending: %w", err))
		return
	}

	slog.Info("publishing first step", "run_id", run.RunID, "step_id", firstStep.StepID)
	if err := p.queue.PublishStep(ctx, queue.StepEvent{RunID: run.RunID, StepID: firstStep.StepID}); err != nil {
		p.failRun(ctx, &run, err)
		return
	}
}

func (p *Planner) failRun(ctx context.Context, run *models.Run, err error) {
	p.store.UpdateRunStatus(ctx, run.RunID, models.RunStatusFailed, err.Error())
	slog.Error("run failed", "run_id", run.RunID, "error", err)
}
