package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerRecordCvsOrchestrateRunCommandBuilder creates a new scheduler_record_cvs_orchestrate_run command
func NewSchedulerRecordCvsOrchestrateRunCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("record-cvs-orchestrate-run")
	builder.WithShort("Append one validated cvs_orchestrate_run_v1 JSON line (stdin) to the orchestrate JSONL log")
	help := clipkg.DynamicHelpBuilder("Append one validated cvs_orchestrate_run_v1 JSON line (stdin) to the orchestrate JSONL log")
	help.WithDescriptionLines("Reads a single JSON object from stdin (one line; schema_version must be cvs_orchestrate_run_v1),")
	help.WithDescriptionLines("validates it, and appends one compact line to .zqk/logs/scheduler/cvs_orchestrate_runs.jsonl")
	help.WithDescriptionLines("unless --jsonl-path overrides the destination.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("This is the in-repo implementation of the durable observability event for")
	help.WithDescriptionLines("scripts/cvs_convergence_orchestrate.sh: the shell builds the JSON (same fields as before) and")
	help.WithDescriptionLines("pipes it here so validation and file append live in Go alongside rollup_status_core.jsonl-style")
	help.WithDescriptionLines("append-only logs. observability.Recorder backends remain optional; the JSONL file is the")
	help.WithDescriptionLines("contract for operators and tooling.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("See docs/architecture/CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md (Appendix D).")
	help.AddExample("Append a line produced by the orchestrate script (normally invoked by the script, not by hand)", "echo '{\"schema_version\":\"cvs_orchestrate_run_v1\",\"ended_at_ms\":1,\"duration_ms\":1,\"cvs_id\":\"CONV-x\",\"exit_code\":0,\"stage\":\"rollup\",\"persist_skipped\":false,\"persist_failed\":false}' | %s scheduler record-cvs-orchestrate-run")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("jsonl-path", "", "", "Override JSONL path (default: .zqk/logs/scheduler/cvs_orchestrate_runs.jsonl under project root)")
	builder.AddBoolFlag("dry-run", "", false, "Validate stdin JSON only; do not append")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
