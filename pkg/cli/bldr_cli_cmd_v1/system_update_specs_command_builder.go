package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemUpdateSpecsCommandBuilder creates a new system_update_specs command
func NewSystemUpdateSpecsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("update-specs")
	builder.WithShort("Bulk update object specifications")
	help := clipkg.DynamicHelpBuilder("Bulk update object specifications")
	help.WithDescriptionLines("Bulk update object specifications to use base trait groups and manage field lifecycle.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("This command helps maintain consistency across object specs by:")
	help.WithDescriptionLines("  - Replacing explicit trait lists with base trait groups (base_object_traits, base_auditable_traits)")
	help.WithDescriptionLines("  - Managing field lifecycle (define a new field; aliases add/create, then modify, deprecate, archive, delete)")
	help.WithDescriptionLines("  - Tracking field versioning and breaking changes")
	help.WithDescriptionLines("  - Validating changes before applying")
	help.WithDescriptionLines("  - Refreshing .zqk/specs/spec_index.json after successful writes (same path as generate-spec-index)")
	help.AddExample("Preview what would be updated (dry-run)", "%s system update-specs --dry-run")
	help.AddExample("Update all specs that extend base_object to use base_object_traits", "%s system update-specs --trait-groups")
	help.AddExample("Update specific spec files", "%s system update-specs --files backlog_item.yaml,goal.yaml")
	help.AddExample("Define a new field (add and create are synonyms)", "%s system update-specs backlog_item --field new_field --operation define --reason \"Adding new field for feature X\"")
	help.AddExample("Field operation then run REQ-019 field-vetting on the written spec", "%s system update-specs goal --field priority --operation define --validate --reason \"…\"")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddBoolFlag("dry-run", "", false, "Preview changes without applying them")
	builder.AddBoolFlag("trait-groups", "", false, "Update specs to use base trait groups")
	builder.AddStringArrayFlag("files", "", "Specific spec files to update (comma-separated)")
	builder.AddBoolFlag("validate", "", false, "Validate specs after updating (trait/file batch) or after a field operation (same REQ-019 path as check --validate-specs)")
	builder.AddStringFlag("field", "", "", "Field name to operate on (for field operations)")
	builder.AddStringFlag("operation", "", "", "Field operation: define (new field; aliases add, create), modify, deprecate, archive, delete")
	builder.AddBoolFlag("breaking-change", "", false, "Mark change as breaking")
	builder.AddStringFlag("reason", "", "", "Reason for the change")
	builder.AddStringFlag("migration-notes", "", "", "Migration guidance for breaking changes")
	builder.AddStringFlag("replaced-by", "", "", "Field that replaces this one (for deprecate/delete)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
