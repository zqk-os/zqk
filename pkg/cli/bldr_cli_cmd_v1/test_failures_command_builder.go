package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewTestFailuresCommandBuilder creates a new test_failures command
func NewTestFailuresCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("test-failures")
	builder.WithShort("List and manage failing tests")
	help := clipkg.DynamicHelpBuilder("List and manage failing tests")
	help.WithDescriptionLines("List and manage failing tests from recent test runs.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Data sources:")
	help.WithDescriptionLines("- Callback logs under .zqk/callbacks (when callbacks are configured)")
	help.WithDescriptionLines("- Scheduler test-bundle shared events: .zqk/logs/scheduler/cvs/test-bundles/events.jsonl")
	help.WithDescriptionLines("- Rolling health timeline: .zqk/logs/scheduler/cvs/test-bundles/health.jsonl")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Failed bundle event lines include test_failures and suggested_rerun_commands (copy-paste go test")
	help.WithDescriptionLines("lines). Subcommands list and rerun consider both callbacks and test-bundle events.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Convergence measurement (health rollup, CVS handoff) lives under `zqk scheduler convergence`")
	help.WithDescriptionLines("(measure, overseer), not under test-failures.")
	help.AddExample("List failing tests (callbacks + test-bundle events)", "%s scheduler test-failures list")
	help.AddExample("List failures for a specific package", "%s scheduler test-failures list --package ./pkg/storage")
	help.AddExample("Re-run only failing tests (creates scheduler jobs)", "%s scheduler test-failures rerun")
	help.AddExample("Summarize test-bundle health from health.jsonl", "%s scheduler test-failures health")
	help.AddExample("Convergence measure for automation (JSON)", "%s scheduler convergence measure --format json")
	help.AddExample("Convergence JSON without literal gate scripts (faster; full gates + matrix in cvs_outcome_rollup.py)", "%s scheduler convergence measure --format json --skip-rollup-gates")
	help.AddExample("Convergence measure with suggested CVS fields for object update", "%s scheduler convergence measure --format json --session-id CVS-001")
	help.AddExample("Convergence with phase router alignment (current CVS phase + flow variant)", "%s scheduler convergence measure --format json --session-id CVS-001 --current-phase c5_verify --flow-variant scheduler_fast")
	help.AddExample("Object update payload only (pipe to file for zqk object update --file)", "%s scheduler convergence measure --format json --session-id CVS-001 | jq '.suggested_convergence_session_fields.object_update_body'")
	help.AddExample("Include iteration tombstone (before_state_snapshot) in object update payload", "%s scheduler convergence measure --format json --session-id CVS-001 --stamp-tombstone | jq '.suggested_convergence_session_fields.object_update_body'")
	help.AddExample("Finalize: merge predictions with retrospective debrief into object update payload", "%s scheduler convergence measure --format json --session-id CVS-001 --finalize-debrief | jq '.suggested_convergence_session_fields.object_update_body.predictions'")
	help.AddExample("Handoff: operator debrief notes on the CVS for the next convergence session", "%s scheduler convergence measure --format json --session-id CVS-001 --debrief-notes 'Next: focus pkg/scheduler first; watch retention bundle timeouts.' | jq '.suggested_convergence_session_fields.object_update_body.debrief_notes'")
	help.AddExample("Agent prompt (markdown for chat) — same command with --format agent-prompt", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001")
	help.AddExample("Agent prompt to file only (no CVS write; omit --persist-session)", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001 -o .zqk/logs/drift/cvs_agent_prompt_latest.md")
	help.AddExample("Agent prompt copied to clipboard (macOS pbcopy)", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001 --copy")
	help.AddExample("Agent prompt then AppleScript: activate IDE, ⌘Y, ⌘V, Return (macOS; Accessibility)", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001 --paste-ide")
	help.AddExample("Agent prompt in test / non-directive mode (banner + machine-parseable comment for queue integration)", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001 --attention-mode test_non_directive")
	help.AddExample("Persist CVS + log file + clipboard for chat (automated loop handoff; macOS)", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001 --persist-session -o .zqk/logs/drift/cvs_agent_prompt_latest.md --copy")
	help.AddExample("Persist CVS measurement then emit agent prompt only (no clipboard; use --copy or --paste-ide for chat)", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001 --persist-session")
	help.AddExample("Coordinator overseer: rollup + nested CVS tree + arbitrated prompt line", "%s scheduler convergence overseer --coordinator-session-id CVS-001 --format json")
	help.AddExample("Analyze failure patterns (optional backlog items)", "%s scheduler test-failures analyze")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
