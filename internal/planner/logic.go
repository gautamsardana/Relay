package planner

import (
	"context"
	"fmt"
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

	slog.Info("generating plan", "workflowID: ", request.WorkflowID)
	steps, err := p.agent.GeneratePlan(ctx, request.Request, p.registry.All())
	if err != nil {
		p.failWorkflow(ctx, request, err)
		return
	}

	slog.Info("validating tools", "workflowID: ", request.WorkflowID)
	for _, step := range steps {
		_, toolExists := p.registry.Get(step.Tool)
		if !toolExists {
			p.failWorkflow(ctx, request, fmt.Errorf("invalid tool used in steps: %s", step.Tool))
			return
		}
	}

	slog.Info("writing steps to store", "workflowID: ", request.WorkflowID)
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
		return
	}

	slog.Info("marking workflow as processing", "workflowID: ", request.WorkflowID)
	err = p.store.UpdateWorkflowStatus(ctx, request.WorkflowID, models.WorkflowStatusProcessing, "")
	if err != nil {
		p.failWorkflow(ctx, request, err)
		return
	}

	slog.Info("publishing first step", "workflowID: ", request.WorkflowID)
	sort.Slice(modelSteps, func(i, j int) bool {
		return modelSteps[i].StepNumber < modelSteps[j].StepNumber
	})
	if len(modelSteps) == 0 {
		p.failWorkflow(ctx, request, fmt.Errorf("no steps defined for this workflow"))
		return
	}
	firstStepID := modelSteps[0].StepID
	
	err = p.queue.PublishStep(ctx, queue.StepEvent{WorkflowID: request.WorkflowID, StepID: firstStepID})
	if err != nil {
		p.failWorkflow(ctx, request, err)
		return
	}
}

func (p *Planner) failWorkflow(ctx context.Context, workflow *models.Workflow, err error) {
    p.store.UpdateWorkflowStatus(ctx, workflow.WorkflowID, models.WorkflowStatusFailed, err.Error())
    slog.Error("workflow failed", "workflow_id", workflow.WorkflowID, "error", err)
}

func (p *Planner) GetStepsByWorkflow(ctx context.Context, workflowID string) ([]models.Step, error) {
	return p.store.ListStepsByWorkflow(ctx, workflowID)
}