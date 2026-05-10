package store

import (
	"context"

	"github.com/gautamsardana/relay/internal/models"
)

func (s *Store) CreateWorkflow(ctx context.Context, mw *models.Workflow) error {
    _, err := s.queries.CreateWorkflow(ctx, fromModelWorkflowCreate(mw))
	if err != nil {
		return err
	}
    return nil
}