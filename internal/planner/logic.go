package planner

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/queue"
	"github.com/google/uuid"
)

func (p *Planner) HandleWorkflow(request *models.Workflow){
	ctx, cancel := context.WithTimeout(context.Background(), 2 * time.Minute)
	defer cancel()

	// 1. generate plan
	steps, err := p.agent.GeneratePlan(ctx, request.Request, p.registry.All())
	if err != nil {
		p.failWorkflow(ctx, request, err)
	}

	// 2. validate all tools are in registry
	for _, step := range steps {
		_, toolExists := p.registry.Get(step.Tool)
		if !toolExists {
			p.failWorkflow(ctx, request, err)
		}
	}

	// 3. insert steps
	modelSteps := make([]models.Step, len(steps))

	for i, step := range steps {
		stepID, _ := uuid.NewV7()
		
		modelSteps[i] = models.Step{
			StepID:      stepID.String(),
			WorkflowID:  request.WorkflowID,
			StepNumber:  step.StepNumber,
			Tool:        step.Tool,
			Description: step.Description,
			Input:       step.Input,
			Status:      models.StepStatusPending,
			RetryCount:  0,
		}
	}

	err = p.store.InsertSteps(ctx, modelSteps)
	if err != nil {
		p.failWorkflow(ctx, request, err)
	}

	// 4. mark workflow as 'processing'
	err = p.store.UpdateWorkflowStatus(ctx, request, string(models.WorkflowStatusProcessing))
	if err != nil {
		p.failWorkflow(ctx, request, err)
	}

	// 5. publish first step
	sort.Slice(modelSteps, func(i, j int) bool {
		return modelSteps[i].StepNumber < modelSteps[j].StepNumber
	})
	firstStepID := modelSteps[0].StepID
	
	err = p.queue.PublishStep(ctx, queue.StepEvent{WorkflowID: request.WorkflowID, StepID: firstStepID})
	if err != nil {
		p.failWorkflow(ctx, request, err)
	}
}

func (p *Planner) failWorkflow(ctx context.Context, workflow *models.Workflow, err error) {
    p.store.UpdateWorkflowStatus(ctx, workflow, string(models.WorkflowStatusFailed))
    slog.Error("workflow failed", "workflow_id", workflow.WorkflowID, "error", err)
}