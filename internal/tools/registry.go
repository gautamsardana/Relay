package tools

import (
	"context"
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
	return nil 
}
func (r *Registry) Register(t Tool) {}

func (r *Registry) Get(name string) (Tool, bool){
	return nil, false
}

// used to build the Claude prompt
func (r *Registry) All() []Tool {
	return nil
}   

func (r *Registry) Names() []string {
	return nil
}