package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

func (gpt *GPT) GeneratePlan(ctx context.Context, request string, currTools []tools.Tool) ([]StepPlan, error) {
    toolDescriptions := tools.BuildToolDescriptions(currTools)

    prompt := fmt.Sprintf(`You are a workflow planning assistant. Given a goal and a list of available tools, generate a step-by-step execution plan.

	Available tools:
	%s

	Goal: %s

	Return ONLY a JSON array of steps. No explanation, no markdown, no code blocks. Just the raw JSON array.

	Each step must have exactly these fields:
	- step_number: integer starting from 1
	- tool: must be exactly one of the tool names listed above
	- description: what this step does in plain English
	- input: object with the input parameters for the tool

	For steps that depend on previous step outputs, use template syntax: {{steps[N].output.FIELD}} where N is the step_number.

	Example output:
	[
	{"step_number": 1, "tool": "web_search", "description": "Search for software engineering jobs", "input": {"query": "latest software engineering jobs US 2026"}},
	{"step_number": 2, "tool": "notion_write", "description": "Post results to Notion", "input": {"content": "{{steps[1].output.results}}"}}
	]`, toolDescriptions, request)

    resp, err := gpt.Client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
        Model: openai.GPT4o,
        Messages: []openai.ChatCompletionMessage{
            {
                Role:    openai.ChatMessageRoleSystem,
                Content: "You are a workflow planning assistant. You only respond with valid JSON arrays. Never include explanation or markdown.",
            },
            {
                Role:    openai.ChatMessageRoleUser,
                Content: prompt,
            },
        },
        Temperature: 0,
    },
    )
    if err != nil {
        return nil, fmt.Errorf("GPT completion error: %w", err)
    }

    content := resp.Choices[0].Message.Content

    var steps []StepPlan
    if err := json.Unmarshal([]byte(content), &steps); err != nil {
        return nil, fmt.Errorf("failed to parse plan: %w, raw response: %s", err, content)
    }

    return steps, nil
}

func (gpt *GPT) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
    resp, err := gpt.Client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
        Model: openai.GPT4o,
        Messages: []openai.ChatCompletionMessage{
            {Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
            {Role: openai.ChatMessageRoleUser, Content: userPrompt},
        },
        Temperature: 0,
    })
    if err != nil {
        return "", fmt.Errorf("GPT completion error: %w", err)
    }
    if len(resp.Choices) == 0 {
        return "", fmt.Errorf("GPT: empty response")
    }
    return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

