package store

import (
	"context"
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