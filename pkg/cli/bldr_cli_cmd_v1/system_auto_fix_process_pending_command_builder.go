package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemAutoFixProcessPendingCommandBuilder creates a new system_auto_fix_process_pending command
func NewSystemAutoFixProcessPendingCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("auto-fix-process-pending")
	builder.WithShort("Process all pending autofix batch files in .zqk/autofix/ (internal, scheduler)")
	help := clipkg.DynamicHelpBuilder("Process all pending autofix batch files in .zqk/autofix/ (internal, scheduler)")
	help.WithDescriptionLines("Discovers all AUTOFIX-*.json under .zqk/autofix/ and processes each via the standardized")
	help.WithDescriptionLines("pipeline (INGEST → NORMALIZE → COMMIT → FINALIZE) per docs/architecture/data-pipeline-lifecycle.md.")
	help.WithDescriptionLines("Used by SCH-autofix-process-pending so autofix files don't build up; can be triggered when")
	help.WithDescriptionLines("batches are submitted or run on a timer. Internal command (hidden from main help).")
	help.AddExample("Process pending (scheduler or manual)", "%s system auto-fix-process-pending")
	help.AddExample("With explicit project root", "%s system auto-fix-process-pending --project-root /path/to/project")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("project-root", "", "", "Project root (default: resolve from current directory)")
	builder.AddBoolFlag("allow-degraded", "", false, "Allow running when scheduler daemon is not running (results may be partial/stale)")
	builder.AddBoolFlag("sync-glossary", "", false, "After processing batches, run system sync-glossary-from-specs (dry-run report; organele maintenance)")
	builder.AddBoolFlag("sync-glossary-apply", "", false, "With --sync-glossary, also create missing glossary terms (guarded by AUTOFIX_GLOSSARY_MAX_CREATE; requires explicit enable)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
