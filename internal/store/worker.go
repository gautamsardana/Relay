package store

import (
	"context"
	"time"

	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/store/sqlc"
)

func (s *Store) CreateWorker(ctx context.Context, worker *models.Worker) (models.Worker, error) {
	row, err := s.queries.CreateWorker(ctx, fromModelWorkerCreate(worker))
	if err != nil {
		return models.Worker{}, err
	}
	return toModelWorker(&row), nil
}

func (s *Store) GetWorkerByID(ctx context.Context, workerID string) (models.Worker, error) {
	row, err := s.queries.GetWorkerByID(ctx, workerID)
	if err != nil {
		return models.Worker{}, err
	}
	return toModelWorker(&row), nil
}

func (s *Store) ListWorkersByUser(ctx context.Context, userID string) ([]models.Worker, error) {
	rows, err := s.queries.ListWorkersByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	workers := make([]models.Worker, 0, len(rows))
	for _, row := range rows {
		r := row
		workers = append(workers, toModelWorker(&r))
	}
	return workers, nil
}

func (s *Store) ListDueWorkers(ctx context.Context) ([]models.Worker, error) {
	rows, err := s.queries.ListDueWorkers(ctx)
	if err != nil {
		return nil, err
	}
	workers := make([]models.Worker, 0, len(rows))
	for _, row := range rows {
		r := row
		workers = append(workers, toModelWorker(&r))
	}
	return workers, nil
}

func (s *Store) UpdateWorkerNextRunAt(ctx context.Context, workerID string, nextRunAt *time.Time) error {
	return s.queries.UpdateWorkerNextRunAt(ctx, sqlc.UpdateWorkerNextRunAtParams{
		WorkerID:  workerID,
		NextRunAt: fromTimePtr(nextRunAt),
	})
}

func (s *Store) UpdateWorkerStatus(ctx context.Context, workerID string, status models.WorkerStatus) error {
	return s.queries.UpdateWorkerStatus(ctx, sqlc.UpdateWorkerStatusParams{
		WorkerID: workerID,
		Status:   fromModelWorkerStatus(status),
	})
}
