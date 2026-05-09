package agent

import (
	"context"
	"fmt"

	"github.com/gautamsardana/relay/internal/config"
	"github.com/gautamsardana/relay/internal/tools"
	"github.com/sashabaranov/go-openai"
)

type GPT struct {
	Client *openai.Client
}

func NewGPT(config *config.Config) (*GPT, error){
	if config.Env.GPTApiKey == "" {
		return nil, fmt.Errorf("Missing GPT API KEY")
	} 
	return &GPT{Client: openai.NewClient(config.Env.GPTApiKey)}, nil
}

func (gpt *GPT) GeneratePlan(ctx context.Context, request string, tools []tools.Tool) ([]StepPlan, error){
	return nil, nil
}

