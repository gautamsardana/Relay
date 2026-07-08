package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/gautamsardana/relay/internal/config"
	"github.com/gautamsardana/relay/internal/store"
)

// Run-time data needed for the tools for dedup and scoring
type ExecutionContext struct {
    RunID         string
    WorkerID      string
    Instructions  string
    Category      string
    Keywords      []string
    LocationPref  string
    Level         string
    ResumeText    string
    RecencyWeight int
}

type Tool interface {
    Name()        string
    Description() string
    Execute(ctx context.Context, input map[string]any, exec ExecutionContext) (map[string]any, error)
}

type Registry struct {
    tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool){
	for _, tool := range r.tools {
		if tool.Name() == name {
			return tool, true
		}
	}
	return nil, false
}

func (r *Registry) All() []Tool {
	var tools []Tool
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}   

func (r *Registry) Names() []string {
	var names []string
	for _, tool := range r.tools {
		names = append(names, tool.Name())
	}
	return names
}

func BuildToolDescriptions(tools []Tool) string {
    var sb strings.Builder
    for _, t := range tools {
        sb.WriteString(fmt.Sprintf("- %s: %s\n", t.Name(), t.Description()))
    }
    return sb.String()
}

// BuildRegistry constructs the full tool registry shared by the API (for
// planning/validation) and the executor (for execution). Centralizing it ensures
// both binaries register an identical tool set — a mismatch would surface as a
// "tool not in registry" failure at execution time.
func BuildRegistry(cfg *config.Config, s *store.Store, llm Completer) *Registry {
    r := NewRegistry()
    r.Register(NewWebSearch(cfg))
    r.Register(NewHTTPRequest())
    r.Register(NewDocumentRead())
    r.Register(NewJobSearch(s))
    r.Register(NewScoreJobs(s, llm))
    return r
}