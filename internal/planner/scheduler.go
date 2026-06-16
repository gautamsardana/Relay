package planner

import (
	"context"
	"log/slog"
	"time"

	"github.com/gautamsardana/relay/internal/models"
)

const schedulerInterval = 60 * time.Second

// StartScheduler runs a ticker that fires runs for workers whose next_run_at
// has elapsed. Interval-based only for now: after a worker fires, its next run
// is scheduled one interval into the future.
//
// NOTE: assumes a single API instance. Running multiple instances would cause
// duplicate fires for the same due worker until we add an atomic claim
// (e.g. UPDATE ... WHERE next_run_at <= NOW() ... RETURNING / SKIP LOCKED).
func (p *Planner) StartScheduler() {
	ticker := time.NewTicker(schedulerInterval)
	go func() {
		for range ticker.C {
			p.scheduleDueWorkers()
		}
	}()
	slog.Info("scheduler started", "interval", schedulerInterval)
}

func (p *Planner) scheduleDueWorkers() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	due, err := p.store.ListDueWorkers(ctx)
	if err != nil {
		slog.Error("scheduler: failed to fetch due workers", "error", err)
		return
	}

	if len(due) == 0 {
		return
	}

	slog.Info("scheduler: found due workers", "count", len(due))
	for _, worker := range due {
		p.scheduleWorker(ctx, worker)
	}
}

func (p *Planner) scheduleWorker(ctx context.Context, worker models.Worker) {
	runID, err := p.CreateRun(ctx, worker.WorkerID)
	if err != nil {
		// Log but still advance next_run_at below, otherwise the worker stays
		// due and we'd retry every tick.
		slog.Error("scheduler: failed to create run", "worker_id", worker.WorkerID, "error", err)
	} else {
		slog.Info("scheduler: run created", "worker_id", worker.WorkerID, "run_id", runID)
	}

	next := time.Now().Add(time.Duration(worker.IntervalSeconds) * time.Second)
	if err := p.store.UpdateWorkerNextRunAt(ctx, worker.WorkerID, &next); err != nil {
		slog.Error("scheduler: failed to advance next_run_at", "worker_id", worker.WorkerID, "error", err)
	}
}
