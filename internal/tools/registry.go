package tools

import (
	"context"
	"fmt"
	"strings"
)

type Tool interface {
    Name()        string
    Description() string
    Execute(ctx context.Context, input map[string]any) (map[string]any, error)
}

type Registry struct {
    tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{}
}

func (r *Registry) Register(t Tool) {
	r.tools = make(map[string]Tool)
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