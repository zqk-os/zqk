package cli

import (
	"fmt"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/spf13/cobra"
)

// QuarantinedCommand represents a command that exists but is not yet ready
type QuarantinedCommand struct {
	Name        string
	Description string
	Reason      string
	PlannedFor  string // e.g., "v1.1.0", "Q2 2025"
}

// CreateQuarantinedCommand creates a command that shows a "not yet implemented" message
func CreateQuarantinedCommand(qc QuarantinedCommand) *cobra.Command {
	return &cobra.Command{
		Use:   qc.Name,
		Short: qc.Description,
		Long: fmt.Sprintf(`%s

⚠️  This command is not yet implemented.

Reason: %s
Planned for: %s

See the CLI ontology specification for planned functionality.`,
			qc.Description, qc.Reason, qc.PlannedFor),
		RunE: func(cmd *cobra.Command, args []string) error {
			return errfmt.Errorf("command '%s' is not yet implemented (planned for %s)", qc.Name, qc.PlannedFor)
		},
		Hidden: true, // Hide from help by default
	}
}

// QuarantinedCommands is a registry of commands that are planned but not implemented
var QuarantinedCommands = []QuarantinedCommand{
	// Add commands here as they are planned but not ready
	// Example:
	// {
	// 	Name:        "create",
	// 	Description: "Create a new object",
	// 	Reason:      "Object storage backend not yet implemented",
	// 	PlannedFor:  "v1.0.0",
	// },
}

// RegisterQuarantinedCommands registers all quarantined commands to the root
func RegisterQuarantinedCommands(rootCmd *cobra.Command) {
	for _, qc := range QuarantinedCommands {
		rootCmd.AddCommand(CreateQuarantinedCommand(qc))
	}
}
