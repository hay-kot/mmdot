package actions2

import (
	"fmt"

	"github.com/hay-kot/mmdot/internal/core"
)

// registry holds all registered action factories
var registry = make(map[core.ActionType]core.ActionFactory)

// Register adds an action factory to the registry
func Register(name core.ActionType, factory core.ActionFactory) {
	registry[name] = factory
}

// Create instantiates an ActionExecutor from an Action using the registry
func Create(action core.Action) (core.ActionExecutor, error) {
	factory, ok := registry[action.Name]
	if !ok {
		return nil, fmt.Errorf("unknown action type: %s", action.Name)
	}

	return factory(action.Body)
}