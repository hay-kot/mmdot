package core

import (
	"context"
	"time"

	"github.com/BurntSushi/toml"
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

// ActionFactory is a function that creates an ActionExecutor from a TOML primitive
type ActionFactory func(toml.Primitive, toml.MetaData) (ActionExecutor, error)

type Action struct {
	Name ActionType `toml:"name"`
	Body toml.Primitive
}

// UnmarshalTOML implements custom unmarshaling for Action
func (a *Action) UnmarshalTOML(data interface{}) error {
	// Create a temporary struct to decode just the name
	var temp struct {
		Name ActionType `toml:"name"`
	}

	// First, decode just the name field
	md := toml.MetaData{}
	if err := md.PrimitiveDecode(data.(toml.Primitive), &temp); err != nil {
		return err
	}

	// Set the name
	a.Name = temp.Name

	// Store the entire primitive for later decoding
	a.Body = data.(toml.Primitive)

	return nil
}
