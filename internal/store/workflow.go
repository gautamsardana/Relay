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

func (s *Store) GetWorkflow(ctx context.Context, workflowID string) (models.Workflow, error) {
	workflow, err := s.queries.GetWorkflowById(ctx, workflowID)
	if err != nil {
		return models.Workflow{}, err
	}

	return models.Workflow{
		WorkflowID: workflow.WorkflowID,
		Request:    workflow.Request,
		Status:     workflow.Status,
		Error:      workflow.Error.String,
		CreatedAt:  workflow.CreatedAt,
		UpdatedAt:  workflow.UpdatedAt,
	}, nil
}

func (s *Store) UpdateWorkflowStatus(ctx context.Context, workflowID string, newStatus models.WorkflowStatus, errMsg string) error {
	err := s.queries.UpdateWorkflowStatus(ctx, fromModelWorkflowUpdateStatus(workflowID, newStatus, errMsg))
	return err
}
