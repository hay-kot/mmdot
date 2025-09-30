package core

import (
	"context"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
)

type ActionType string

const (
	ActionTypeTemplate = "template"
)

// ActionExecutor defines the interface that all actions must implement
type ActionExecutor interface {
	Execute(ctx context.Context, env *ExecutionEnv) (*ActionResult, error)
	Validate() error
}

// ExecutionEnv contains the runtime environment for action execution
type ExecutionEnv struct {
	Config    ConfigFile
	Variables map[string]any
}

// ActionResult represents the outcome of executing an action
type ActionResult struct {
	Action   ActionType
	Changed  bool
	Error    error
	Duration time.Duration
}

// ActionFactory is a function that creates an ActionExecutor from a YAML node
type ActionFactory func(ast.Node) (ActionExecutor, error)

type Action struct {
	Name ActionType `yaml:"name"`
	Body ast.Node
}

// UnmarshalYAML implements custom unmarshaling for Action
func (a *Action) UnmarshalYAML(node ast.Node) error {
	// Decode just the name field
	var temp struct {
		Name ActionType `yaml:"name"`
	}

	if err := yaml.NodeToValue(node, &temp); err != nil {
		return err
	}

	// Set the name
	a.Name = temp.Name

	// Store the entire node for later decoding
	a.Body = node

	return nil
}
