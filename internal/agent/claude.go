package agent

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/gautamsardana/relay/internal/config"
	"github.com/gautamsardana/relay/internal/tools"
)

type Claude struct {
	Client *anthropic.Client
}

func NewClaude(config *config.Config) ( *Claude, error){
	if config.Env.ClaudeApiKey == "" {
		return nil, fmt.Errorf("Missing Claude API KEY")
	} 

	client := anthropic.NewClient(option.WithAPIKey(config.Env.ClaudeApiKey))
	return &Claude{Client: &client}, nil
}

func (c *Claude) GeneratePlan(ctx context.Context, request string, tools []tools.Tool) ([]StepPlan, error){
	return nil, nil
}