package agent

import (
	"context"
	// "fmt"
	"log"

	"github.com/gautamsardana/relay/internal/config"
	"github.com/gautamsardana/relay/internal/tools"
)

type AgentManager struct {
    primary   Agent
    secondary Agent
}

func NewAgentManager(config *config.Config) (*AgentManager, error) {
    // if config.App.AIPrimary == config.App.AISecondary {
	// 	return nil, fmt.Errorf("primary and secondary AI providers must be different")
    // }

	primary, err := newAgent(config, config.App.AIPrimary)
	if err != nil {
		return nil, err
	}

	secondary, err := newAgent(config, config.App.AISecondary)
	if err != nil {
		return nil, err
	}

    return &AgentManager{
        primary:   primary,
        secondary: secondary,
    }, nil
}

func newAgent(config *config.Config, provider string) (Agent, error) {
    switch provider {
    case "anthropic":
        return NewClaude(config)
    case "openai":
        return NewGPT(config)
    case "groq":
        return NewGroq(config)
    default:
        // log.Fatalf("unsupported AI provider: %s", provider)
        // return nil
		return NewFAaaa()
    }
}

func (am *AgentManager) GeneratePlan(ctx context.Context, request string, tools []tools.Tool) ([]StepPlan, error) {
    plan, err := am.primary.GeneratePlan(ctx, request, tools)
    if err != nil {
        log.Printf("primary agent failed: %v, falling back to secondary", err)
        return am.secondary.GeneratePlan(ctx, request, tools)
    }
    return plan, nil
}