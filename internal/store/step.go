package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gautamsardana/relay/internal/models"
)

func (s *Store) InsertSteps(ctx context.Context, ms []models.Step) error {
	tx, err := s.Conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)

	for _, step := range ms {
		_, err := qtx.CreateStep(ctx, fromModelStepCreate(&step))
		if err != nil {
			return fmt.Errorf("failed to insert step %d: %w", step.StepNumber, err)
		}
	}

	return tx.Commit()
}


func (s *Store) ListStepsByRun(ctx context.Context, runID string) ([]models.Step, error) {
	steps, err := s.queries.ListStepsByRun(ctx, runID)
	if err != nil {
		return nil, err
	}

	items := make([]models.Step, 0, len(steps))
	for _, step := range steps {
		stepCopy := step
		items = append(items, toModelStep(&stepCopy))
	}
	return items, nil
}

func (s *Store) UpdateStepStatus(ctx context.Context, stepID string, newStatus models.StepStatus, errMsg string) error {
	err := s.queries.UpdateStepStatus(ctx, fromModelStepUpdateStatus(stepID, newStatus, errMsg))
	return err
}

// ClaimStep atomically marks a step as processing only if it is still pending.
// Returns (step, true, nil) if claimed, (zero, false, nil) if another worker got there first.
func (s *Store) ClaimStep(ctx context.Context, stepID string) (models.Step, bool, error) {
	step, err := s.queries.ClaimStep(ctx, stepID)
	if err == sql.ErrNoRows {
		return models.Step{}, false, nil
	}
	if err != nil {
		return models.Step{}, false, err
	}
	return toModelStep(&step), true, nil
}

func (s *Store) GetStuckSteps(ctx context.Context, timeout time.Duration) ([]models.Step, error) {
	rows, err := s.queries.GetStuckSteps(ctx, int32(timeout.Seconds()))
	if err != nil {
		return nil, err
	}
	steps := make([]models.Step, 0, len(rows))
	for _, row := range rows {
		r := row
		steps = append(steps, toModelStep(&r))
	}
	return steps, nil
}

func (s *Store) CancelUnstartedSteps(ctx context.Context, runID string, reason string) error {
	return s.queries.CancelUnstartedSteps(ctx, fromModelCancelUnstartedSteps(runID, reason))
}

func (s *Store) MarkStepPending(ctx context.Context, stepID string) error {
	return s.queries.MarkStepPending(ctx, stepID)
}

func (s *Store) ResetStepToPending(ctx context.Context, stepID string) error {
	return s.queries.ResetStepToPending(ctx, stepID)
}

func (s *Store) IncrementStepRetryCount(ctx context.Context, stepID string) error {
	return s.queries.IncrementStepRetryCount(ctx, stepID)
}

func (s *Store) UpdateStepAsCompleted(ctx context.Context, stepID string, output map[string]any) error {
	err := s.queries.UpdateStepAsCompleted(ctx, fromModelStepUpdateAsCompleted(stepID, output))
	return err
}

func (s *Store) GetStepByRunAndNumber(ctx context.Context, runID string, stepNumber int) (models.Step, bool, error) {
	step, err := s.queries.GetStepByRunAndNumber(ctx, fromRunStepNumber(runID, int32(stepNumber)))
	if err == sql.ErrNoRows {
		return models.Step{}, false, nil
	}
	if err != nil {
		return models.Step{}, false, err
	}
	return toModelStep(&step), true, nil
}
