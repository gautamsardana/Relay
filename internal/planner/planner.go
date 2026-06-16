package planner

import (
	"context"

	"github.com/google/uuid"

	"github.com/gautamsardana/relay/internal/agent"
	"github.com/gautamsardana/relay/internal/config"
	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/queue"
	"github.com/gautamsardana/relay/internal/store"
	"github.com/gautamsardana/relay/internal/tools"
)

type Planner struct {
	store      *store.Store
	queue      *queue.QueueManager
	agent      *agent.AgentManager
	registry   *tools.Registry
	maxRetries int
}

func New(cfg *config.Config, s *store.Store, q *queue.QueueManager, a *agent.AgentManager, r *tools.Registry) *Planner {
	return &Planner{
		store:      s,
		queue:      q,
		agent:      a,
		registry:   r,
		maxRetries: cfg.App.MaxStepRetries,
	}
}

// CreateRun starts a new run for the given worker: it persists the
// run, then asynchronously plans and publishes its first step.
func (p *Planner) CreateRun(ctx context.Context, workerID string) (string, error) {
	worker, err := p.store.GetWorkerByID(ctx, workerID)
	if err != nil {
		return "", err
	}

	id, _ := uuid.NewV7()

	run := &models.Run{
		RunID:    id.String(),
		WorkerID: workerID,
		Status:   models.RunStatusInit,
	}

	createdRun, err := p.store.CreateRun(ctx, run)
	if err != nil {
		return "", err
	}

	go p.HandleRun(worker, createdRun)

	return createdRun.RunID, nil
}

func (p *Planner) GetStepsByRun(ctx context.Context, runID string) ([]models.Step, error) {
	return p.store.ListStepsByRun(ctx, runID)
}

func (p *Planner) GetRun(ctx context.Context, runID string) (models.Run, error) {
	return p.store.GetRunByID(ctx, runID)
}
