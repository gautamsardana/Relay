package store

import (
	"context"

	"github.com/gautamsardana/relay/internal/models"
)

func (s *Store) CreateRun(ctx context.Context, run *models.Run) (models.Run, error) {
	row, err := s.queries.CreateRun(ctx, fromModelRunCreate(run))
	if err != nil {
		return models.Run{}, err
	}
	return toModelRun(&row), nil
}

func (s *Store) GetRunByID(ctx context.Context, runID string) (models.Run, error) {
	row, err := s.queries.GetRunByID(ctx, runID)
	if err != nil {
		return models.Run{}, err
	}
	return toModelRun(&row), nil
}

func (s *Store) ListRunsByWorker(ctx context.Context, workerID string) ([]models.Run, error) {
	rows, err := s.queries.ListRunsByWorker(ctx, workerID)
	if err != nil {
		return nil, err
	}
	runs := make([]models.Run, 0, len(rows))
	for _, row := range rows {
		r := row
		runs = append(runs, toModelRun(&r))
	}
	return runs, nil
}

func (s *Store) UpdateRunStatus(ctx context.Context, runID string, status models.RunStatus, errMsg string) error {
	return s.queries.UpdateRunStatus(ctx, fromModelRunUpdateStatus(runID, status, errMsg))
}
