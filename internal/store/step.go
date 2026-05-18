package store

import (
	"context"
	"database/sql"
	"fmt"

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
func (s *Store) GetStepByID(ctx context.Context, stepID string) (models.Step, error) {
	step, err := s.queries.GetStepById(ctx, stepID)
	if err != nil {
		return models.Step{}, err
	}
	return toModelStep(&step), nil
}

func (s *Store) UpdateStepStatus(ctx context.Context, stepID string, newStatus models.StepStatus, errMsg string) error {
	err := s.queries.UpdateStepStatus(ctx, fromModelStepUpdateStatus(stepID, newStatus, errMsg))
    return err
}

func (s *Store) UpdateStepAsCompleted(ctx context.Context, stepID string, output map[string]any) error {
	err := s.queries.UpdateStepAsCompleted(ctx, fromModelStepUpdateAsCompleted(stepID, output))
    return err
}

func (s *Store) GetNextStep(ctx context.Context, workflowID string, stepNumber int32) (models.Step, bool, error) {
    nextStep, err := s.queries.GetNextStep(ctx, fromModelStepGetNextStep(workflowID, stepNumber))
    if err == sql.ErrNoRows {
        return models.Step{}, false, nil // no next step, workflow done
    }
    if err != nil {
        return models.Step{}, false, err
    }
    return models.Step{WorkflowID: nextStep.WorkflowID, StepID: nextStep.StepID}, true, nil
}