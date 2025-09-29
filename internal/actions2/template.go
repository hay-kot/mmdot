package actions2

import (
	"context"
	"fmt"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hay-kot/mmdot/internal/core"
)

func init() {
	Register(core.ActionTypeTemplate, NewTemplateAction)
}

type TemplateAction struct {
	Name        core.ActionType `toml:"name"`
	Template    string          `toml:"template"`    // Inline template or file path
	Destination string          `toml:"destination"` // path
	Mode        string          `toml:"mode"`        // Permissions to render as
	Vars        map[string]any  `toml:"vars"`
}

// NewTemplateAction creates a new TemplateAction from a TOML primitive
func NewTemplateAction(primitive toml.Primitive, md toml.MetaData) (core.ActionExecutor, error) {
	var action TemplateAction
	if err := md.PrimitiveDecode(primitive, &action); err != nil {
		return nil, fmt.Errorf("failed to decode template action: %w", err)
	}
	return &action, nil
}

// Execute implements the ActionExecutor interface
func (t *TemplateAction) Execute(ctx context.Context, env *core.ExecutionEnv) (*core.ActionResult, error) {
	start := time.Now()
	result := &core.ActionResult{
		Action:   core.ActionTypeTemplate,
		Duration: time.Since(start),
	}

	// TODO: Implement template rendering logic
	// - Load template from file or use inline
	// - Resolve variables from env.Variables
	// - Render template
	// - Write to destination with correct mode
	// - Set result.Changed based on whether file was modified

	return result, nil
}

// Validate implements the ActionExecutor interface
func (t *TemplateAction) Validate() error {
	if t.Template == "" {
		return fmt.Errorf("template is required")
	}
	if t.Destination == "" {
		return fmt.Errorf("destination is required")
	}
	return nil
}
