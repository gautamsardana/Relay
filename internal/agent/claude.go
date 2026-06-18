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

// Complete is not yet implemented for Claude (deferred, like GeneratePlan).
// Use openai/groq as the primary provider for tools that need completions.
func (c *Claude) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return "", fmt.Errorf("claude: Complete not implemented")
}