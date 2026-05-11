package planner

import (
	"context"

	"github.com/google/uuid"

	"github.com/gautamsardana/relay/internal/agent"
	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/queue"
	"github.com/gautamsardana/relay/internal/store"
	"github.com/gautamsardana/relay/internal/tools"
)

type Planner struct {
    store    *store.Store
	queue    *queue.QueueManager
    agent    *agent.AgentManager
    registry *tools.Registry
}

func New(s *store.Store, q *queue.QueueManager, a *agent.AgentManager, r *tools.Registry) *Planner{
	return &Planner {
		store: s,
		queue: q,
		agent: a,
		registry: r,
	}
}

func (p *Planner) CreateWorkflow(ctx context.Context, requestString string) (string, error){
	id, _ := uuid.NewV7()

	request := &models.Workflow{
		WorkflowID: id.String(),
		Request: requestString, 
		Status: models.WorkflowStatusInit,
	}
	
	err := p.store.CreateWorkflow(ctx, request)
	if err != nil {
		return "", err
	}
	
	go p.HandleWorkflow(request)

	return request.WorkflowID, nil
}