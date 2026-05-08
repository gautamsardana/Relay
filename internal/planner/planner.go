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
    agent    agent.Agent
    registry *tools.Registry
}

func (p *Planner) HandleRequest(ctx context.Context, request string) (string, error){
	return "", nil
}