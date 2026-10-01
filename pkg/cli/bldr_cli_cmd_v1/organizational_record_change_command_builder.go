package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewOrganizationalRecordChangeCommandBuilder creates a new organizational_record_change command
func NewOrganizationalRecordChangeCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("record-change")
	builder.WithShort("Record an organizational change (creates organizational_change object)")
	help := clipkg.DynamicHelpBuilder("Record an organizational change (creates organizational_change object)")
	help.WithDescriptionLines("Record an organizational change by creating an organizational_change object.")
	help.WithDescriptionLines("Use with \"organizational analyze-impact\" to identify affected kernel objects.")
	help.WithDescriptionLines("Use after structure sync or manual edits to track restructures, team moves, etc.")
	help.AddExample("Record a division restructure", "%s organizational record-change --id OCH-001 --change-type division_restructure --title \"Split Engineering\"")
	help.AddExample("Record with description and affected objects from file", "%s organizational record-change --id OCH-002 --change-type team_reassignment --title \"Backend team move\" --description \"Moved to Platform\" --affected-objects affected.yaml")
	help.AddExample("Dry run", "%s organizational record-change --id OCH-001 --change-type division_restructure --title Split --dry-run")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("id", "", "", "Organizational change object ID (e.g. OCH-001)")
	builder.AddStringFlag("change-type", "", "", "Type of change (e.g. division_restructure, team_reassignment, organization_merge, team_move)")
	builder.AddStringFlag("title", "", "", "Short title for the change")
	builder.AddStringFlag("description", "", "", "Human-readable description of the change")
	builder.AddStringFlag("affected-objects", "", "", "Path to YAML/JSON file with map of affected object types to IDs")
	builder.AddStringFlag("change-date", "", "", "ISO-8601 date/time when change occurred (default: now)")
	builder.AddBoolFlag("dry-run", "", false, "Validate only; do not create")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
