package planner

import (
	"context"

	"github.com/google/uuid"

	"github.com/gautamsardana/relay/internal/models"
)

func (p *Planner) CreateWorker(ctx context.Context, userID, name, instructions, schedule, resumeURL string) (models.Worker, error) {
	id, _ := uuid.NewV7()

	worker := &models.Worker{
		WorkerID:     id.String(),
		UserID:       userID,
		Name:         name,
		Instructions: instructions,
		Schedule:     schedule,
		Status:       models.WorkerStatusActive,
		ResumeURL:    resumeURL,
	}

	return p.store.CreateWorker(ctx, worker)
}

func (p *Planner) GetWorker(ctx context.Context, workerID string) (models.Worker, error) {
	return p.store.GetWorkerByID(ctx, workerID)
}

func (p *Planner) ListWorkersByUser(ctx context.Context, userID string) ([]models.Worker, error) {
	return p.store.ListWorkersByUser(ctx, userID)
}

func (p *Planner) ListRunsByWorker(ctx context.Context, workerID string) ([]models.Run, error) {
	return p.store.ListRunsByWorker(ctx, workerID)
}

func (p *Planner) UpdateWorkerStatus(ctx context.Context, workerID string, status models.WorkerStatus) error {
	return p.store.UpdateWorkerStatus(ctx, workerID, status)
}
