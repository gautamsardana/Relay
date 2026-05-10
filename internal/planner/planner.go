package planner

import (
	"context"

	"github.com/gautamsardana/relay/internal/store"
	"github.com/gautamsardana/relay/internal/agent"
	"github.com/gautamsardana/relay/internal/queue"
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

func (p *Planner) CreateWorkflow(ctx context.Context, request string) (string, error){
	return "", nil
}