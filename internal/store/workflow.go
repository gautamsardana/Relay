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

func (s *Store) UpdateWorkflowStatus(ctx context.Context, mw *models.Workflow, newStatus string) error {
    err := s.queries.UpdateWorkflowStatus(ctx, fromModelWorkflowUpdateStatus(mw))
	if err != nil {
		return err
	}
    return nil
}