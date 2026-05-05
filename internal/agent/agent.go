package agent

import (
	"context"

	"github.com/gautamsardana/relay/internal/tools"
)

type StepPlan struct {
    StepNumber  int            `json:"step_number"`
    Tool        string         `json:"tool"`
    Description string         `json:"description"`
    Input       map[string]any `json:"input"`
}

type Agent interface {
    GeneratePlan(ctx context.Context, request string, tools []tools.Tool) ([]StepPlan, error)
}