package planner

import (
	"context"
	"log/slog"
	"time"

	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/queue"
)

const (
	// stuckStepTimeout MUST exceed the executor's per-step work timeout
	// (executor.stepWorkTimeout, 12m). A step waiting out provider rate limits is
	// slow, not dead — reclaiming it before the executor's own deadline would
	// re-run it concurrently with the live attempt, doubling LLM usage and making
	// the rate limit worse. Only reclaim once no healthy executor could still hold it.
	stuckStepTimeout   = 15 * time.Minute
	reconcilerInterval = 60 * time.Second
)

func (p *Planner) StartReconciler() {
	ticker := time.NewTicker(reconcilerInterval)
	go func() {
		for range ticker.C {
			p.reconcile()
		}
	}()
	slog.Info("reconciler started", "interval", reconcilerInterval, "stuck_timeout", stuckStepTimeout)
}

func (p *Planner) reconcile() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stuck, err := p.store.GetStuckSteps(ctx, stuckStepTimeout)
	if err != nil {
		slog.Error("reconciler: failed to fetch stuck steps", "error", err)
		return
	}

	if len(stuck) == 0 {
		return
	}

	slog.Info("reconciler: found stuck steps", "count", len(stuck))

	for _, step := range stuck {
		p.reconcileStep(ctx, step)
	}
}

func (p *Planner) reconcileStep(ctx context.Context, step models.Step) {
	slog.Warn("reconciler: recovering stuck step", "step_id", step.StepID, "run_id", step.RunID, "retry_count", step.RetryCount)

	if err := p.store.IncrementStepRetryCount(ctx, step.StepID); err != nil {
		slog.Error("reconciler: failed to increment retry count", "step_id", step.StepID, "error", err)
		return
	}

	if step.RetryCount+1 >= p.maxRetries {
		slog.Error("reconciler: step exhausted retries, failing run", "step_id", step.StepID, "run_id", step.RunID)
		if err := p.store.UpdateStepStatus(ctx, step.StepID, models.StepStatusFailed, "step exhausted retries"); err != nil {
			slog.Error("reconciler: failed to mark step as failed", "step_id", step.StepID, "error", err)
		}
		if err := p.store.UpdateRunStatus(ctx, step.RunID, models.RunStatusFailed, "step exhausted retries"); err != nil {
			slog.Error("reconciler: failed to mark run as failed", "run_id", step.RunID, "error", err)
		}
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			reason := "cancelled: step " + step.StepID + " exhausted retries"
			if err := p.store.CancelUnstartedSteps(bgCtx, step.RunID, reason); err != nil {
				slog.Warn("reconciler: failed to cancel unstarted steps (non-critical)", "run_id", step.RunID, "error", err)
			}
		}()
		return
	}

	if err := p.store.ResetStepToPending(ctx, step.StepID); err != nil {
		slog.Error("reconciler: failed to reset step to pending", "step_id", step.StepID, "error", err)
		return
	}

	if err := p.queue.PublishStep(ctx, queue.StepEvent{RunID: step.RunID, StepID: step.StepID}); err != nil {
		slog.Error("reconciler: failed to re-publish step", "step_id", step.StepID, "error", err)
		return
	}

	slog.Info("reconciler: step re-queued", "step_id", step.StepID, "attempt", step.RetryCount+1)
}
