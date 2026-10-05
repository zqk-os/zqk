package internal

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// NewInternalUpdateCmd creates an update command for internal/built-in objects
func NewInternalUpdateCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Update an internal or built-in object (admin only)",
		"Update an internal or built-in object with admin privileges.",
		"",
		"This command allows modifying built-in objects that are normally immutable.",
		"Built-in objects are shareable/reusable but can only be modified through",
		"this command with admin privileges.",
		"",
		"The update data can be provided via:",
		"  - --file: Path to a YAML file containing the updates",
		"  - --data: Inline YAML data",
		"  - --field: Update a single field (format: field=value, can be used multiple times)",
		"  - --auto-status: Advance to the next lifecycle-valid status (trait-gated)",
	).
		AddExample("Update a built-in component type", "%s internal update COMP-TYPE-001 --field title=\"Updated Title\"").
		AddExample("Update from file", "%s internal update COMP-TYPE-001 --file updates.yaml").
		AddExample("Update multiple fields", "%s internal update COMP-TYPE-001 --field title=\"New Title\" --field description=\"New Description\"").
		AddExample("Dry-run to see what would be updated", "%s internal update COMP-TYPE-001 --field title=\"New Title\" --dry-run").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use:  "update <id> [flags]",
		Args: cobra.ExactArgs(1),
	}

	cli.BindAsyncProgress(cmd, runInternalUpdate)
	cmd.Flags().String("file", "", "Path to YAML file containing update data")
	cmd.Flags().String("data", "", "Inline YAML data for updates")
	cmd.Flags().StringArray("field", []string{}, "Update a field (field=value). Parses as JSON when valid, else literal string. Repeatable.")
	cmd.Flags().Bool("auto-status", false, "Advance status to the next lifecycle-valid status for this object kind")
	cmd.Flags().Bool("dry-run", false, "Show what would be updated without actually updating")

	return cli.FinalizeCommand(cmd, helpBuilder)
}

func runInternalUpdate(cmd *cobra.Command, args []string) error {
	id, proc, err := initInternalProcessorWithID(cmd, args)
	if err != nil {
		return err
	}

	// Read current object to check if it's built-in
	current, isBuiltIn, err := readCurrentObject(proc, id)
	if err != nil {
		return err
	}

	// Build all updates
	updates, err := buildAllUpdates(cmd, proc, current)
	if err != nil {
		return err
	}

	// Check for dry-run using internal/cli DryRunHandler
	dr := cli.NewDryRunHandler(proc.Logger())
	handled, result, err := dr.HandleUpdateDryRunResult(cmd, id, current, updates)
	if err != nil {
		return err
	}
	if handled && result != nil {
		return cli.FormatOutput(cmd, result)
	}

	// Execute update
	if err := executeUpdate(proc, id, updates); err != nil {
		return err
	}

	// Output success message using shared utility
	return outputUpdateSuccess(cmd, id, isBuiltIn, proc)
}
