package agent

import (
	"context"

	"github.com/gautamsardana/relay/internal/tools"
)

type FakeAgent struct {}

// caller initiates FakeAgent using - var _ Agent = (*FakeAgent)(nil)
func (fa *FakeAgent) GeneratePlan(ctx context.Context, request string, tools []tools.Tool) ([]StepPlan, error){
	return []StepPlan{
        {
            StepNumber: 1,
            Tool: "web_search",
        },
    }, nil
}