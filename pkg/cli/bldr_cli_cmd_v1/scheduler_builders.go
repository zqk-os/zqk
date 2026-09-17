package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

type SchedulerCommandConfig struct {
	Use          string
	Short        string
	Description  []string
	Examples     []string // alternating comment, command
	Flags        func(*clipkg.CommandBuilder)
	ExcludeFlags []string
}

func buildSchedulerCommand(cfg SchedulerCommandConfig) *cobra.Command {
	builder := clipkg.NewCommandBuilder(cfg.Use)
	if cfg.Short != "" {
		builder.WithShort(cfg.Short)
	}

	help := clipkg.DynamicHelpBuilder(cfg.Short)
	for _, desc := range cfg.Description {
		help.WithDescriptionLines(desc)
	}

	for i := 0; i < len(cfg.Examples); i += 2 {
		if i+1 < len(cfg.Examples) {
			help.AddExample(cfg.Examples[i], cfg.Examples[i+1])
		}
	}

	excludedFlags := []string{"format", "output", "verbose", "quiet", "timeout", "columns"}
	if cfg.ExcludeFlags != nil {
		excludedFlags = cfg.ExcludeFlags
	}
	for _, f := range excludedFlags {
		help.ExcludeFlag(f)
	}

	builder.WithHelpBuilder(help)

	if cfg.Flags != nil {
		cfg.Flags(builder)
	}

	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, excludedFlags)
	return builder.Build()
}

// NewSchedulerActivityCommandBuilder creates a new scheduler_activity command
func NewSchedulerActivityCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "activity",
		Short: "Show recent job activity including started but not completed jobs",
		Description: []string{"Show recent scheduler job activity including:",
			"- Data freshness (oldest/newest loaded event) and busyness",
			"- Missed timer triggers with schedule, approximate interval, and severity hints",
			"- Jobs started but not completed (with running-duration and likely-health note)",
			"- Timer jobs: cron field breakdown and link to cron format docs",
			"- One row per job with latest started/completed/failed (relative times)",
			"- Flat recent event list (relative times)",
			"",
			"This helps validate cache refreshes, spot stuck runs, and interpret missed slots in context."},
		Examples: []string{"Show recent activity", "%s scheduler activity",
			"Filter to one job and show more events", "%s scheduler activity --job-id SCH-001 --limit 50",
			"Re-print activity every 10 seconds until interrupted", "%s scheduler activity --watch 10s",
			"Only events from the last 24 hours (duration) or since an instant", "%s scheduler activity --since 24h",
			"Narrow table layout", "%s scheduler activity --compact",
			"Tune stuck-job labels (shorter run = likely healthy, longer = investigate)", "%s scheduler activity --stuck-recent-after 5m --stuck-stale-after 1h"},
		Flags: func(builder *clipkg.CommandBuilder) {
			builder.AddIntFlag("limit", "", 20, "Maximum number of recent events to show")
			builder.AddStringFlag("job-id", "", "", "Filter by specific job ID (optional)")
			builder.AddBoolFlag("bypass-cache", "", false, "Bypass cache and query audit events directly (slower, but more complete for small datasets)")
			builder.AddDurationFlag("watch", "", "0", "If greater than zero, re-print activity after this interval until interrupted (e.g. 10s)")
			builder.AddStringFlag("since", "", "", "Only include events on or after this instant: RFC3339 timestamp, or a duration ago (e.g. 24h, 30m)")
			builder.AddDurationFlag("stuck-recent-after", "", "2m", "For started-but-not-completed: runs shorter than this are labeled likely healthy in-flight")
			builder.AddDurationFlag("stuck-stale-after", "", "30m", "For started-but-not-completed: runs longer than this are labeled investigate")
			builder.AddBoolFlag("compact", "", false, "Use narrower job ID column and table width")
		},
	})
}

// NewSchedulerAnalyzeCommandBuilder creates a new scheduler_ command
func NewSchedulerAnalyzeCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "",
		Short: "Generated spec for analyze",
	})
}

// NewSchedulerBundleProgressCommandBuilder creates a new scheduler_bundle_progress command
func NewSchedulerBundleProgressCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:          "bundle-progress",
		Short:        "Render live test bundle execution progress matrix",
		Examples:     []string{"View live bundle execution progress", "%s scheduler bundle-progress"},
		ExcludeFlags: []string{"output", "verbose", "quiet", "timeout", "columns"},
	})
}

// NewSchedulerClearIssuesCommandBuilder creates a new scheduler_clear_issues command
func NewSchedulerClearIssuesCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "clear-issues",
		Short: "Clear scheduler issues file",
		Description: []string{"Clear the scheduler issues file, setting status to 'ok' and removing all recorded issues.",
			"This manually clears .zqk/scheduler/issues.json. The scheduler will automatically",
			"clear issues when all recorded issues are older than 1 hour, but this command",
			"allows immediate clearing for operational purposes."},
		Examples: []string{"Clear scheduler issues", "%s scheduler clear-issues"},
	})
}

// NewSchedulerConfigCommandBuilder creates a new scheduler_config command
func NewSchedulerConfigCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "config",
		Short: "Show or set scheduler config (jobs_paused)",
		Description: []string{"Show or update scheduler configuration (e.g. jobs_paused).",
			"",
			"Without flags, shows the current config. Use --jobs-paused or --no-jobs-paused to set jobs_paused.",
			"When jobs_paused is true, the scheduler (after restart) will not run timer or immediate jobs;",
			"only manually triggered jobs run. Restart the scheduler for changes to take effect."},
		Examples: []string{"Show current scheduler config", "%s scheduler config",
			"Pause automatic jobs (only manual trigger runs after restart)", "%s scheduler config --jobs-paused",
			"Resume normal scheduling", "%s scheduler config --no-jobs-paused"},
		Flags: func(builder *clipkg.CommandBuilder) {
			builder.AddBoolFlag("jobs-paused", "", false, "Set jobs_paused to true (no timer/immediate jobs; only manual trigger)")
			builder.AddBoolFlag("no-jobs-paused", "", false, "Set jobs_paused to false (normal scheduling)")
		},
	})
}

// NewSchedulerConvergenceCommandBuilder creates a new scheduler_convergence command
func NewSchedulerConvergenceCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "convergence",
		Short: "Convergence measurement (test bundles, rollup, agent handoff)",
		Description: []string{"Test-bundle health timeline, rollup_status_core, coordinator overseer, and optional CVS/agent handoff.",
			"",
			"measure              — Read health.jsonl and compute delta_assessment, rollup, suggested CVS fields (distinct from",
			"                       CVS object fields like before_state_snapshot / after_state_snapshot — those are persisted state).",
			"overseer             — Coordinator/nested CVS rollup (tree walk + arbitrated parent line).",
			"nest-spawn           — Create child CVS under parent (related_object_refs; depth/cycle safe).",
			"nest-link            — Link existing child CVS under parent.",
			"nest-status          — BFS nest tree status for a parent CVS.",
			"promotion-readiness   — Optional promotion gate; delegates to scripts/check_convergence_promotion_readiness.sh (bash + jq).",
			"record-overseer-run   — Append overseer_run_v1 JSONL; delegates to scripts/record_convergence_overseer_run.sh.",
			"",
			"For agent markdown, use measure with --format agent-prompt. For JSON automation, use --format json."},
		Examples: []string{"Convergence measure for automation (JSON)", "%s scheduler convergence measure --format json",
			"Convergence JSON without literal gate scripts (faster; full gates + matrix in cvs_outcome_rollup.py)", "%s scheduler convergence measure --format json --skip-rollup-gates",
			"Convergence measure with suggested CVS fields for object update", "%s scheduler convergence measure --format json --session-id CVS-001",
			"Convergence with phase router alignment (current CVS phase + flow variant)", "%s scheduler convergence measure --format json --session-id CVS-001 --current-phase c5_verify --flow-variant scheduler_fast",
			"Object update payload only (pipe to file for zqk object update --file)", "%s scheduler convergence measure --format json --session-id CVS-001 | jq '.suggested_convergence_session_fields.object_update_body'",
			"Agent prompt (markdown) for chat — same command as JSON, different --format", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001",
			"Agent prompt to file only (no CVS write; omit --persist-session when health watermark unchanged)", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001 -o .zqk/logs/drift/cvs_agent_prompt_latest.md",
			"Agent prompt copied to clipboard (macOS pbcopy)", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001 --copy",
			"Agent prompt then AppleScript: activate Cursor, ⌘Y, ⌘V, Return (macOS; Accessibility)", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001 --paste-cursor",
			"Persist CVS + log file + clipboard for chat (automated loop handoff; macOS)", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001 --persist-session -o .zqk/logs/drift/cvs_agent_prompt_latest.md --copy",
			"Coordinator overseer: rollup + nested CVS tree + arbitrated prompt line", "%s scheduler convergence overseer --coordinator-session-id CVS-001 --format json",
			"Nest status BFS tree", "%s scheduler convergence nest-status --parent-session-id CVS-001 --format json",
			"Promotion readiness gate (session id arg or env; same exit codes as shell script)", "%s scheduler convergence promotion-readiness CVS-001",
			"Record one overseer run JSONL line (coordinator id arg or COORDINATOR_SESSION_ID)", "%s scheduler convergence record-overseer-run CVS-001"},
	})
}

// NewSchedulerCriteriaEvidenceCommandBuilder creates a new scheduler_ command
func NewSchedulerCriteriaEvidenceCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "",
		Short: "Generated spec for criteria-evidence",
	})
}

// NewSchedulerDumpCommandBuilder creates a new scheduler_dump command
func NewSchedulerDumpCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "dump",
		Short: "Capture process dump from the running scheduler daemon",
		Description: []string{"Sends SIGUSR1 to the scheduler daemon to capture goroutine dump, heap profile,",
			"and thread info to .zqk/scheduler/diagnostics/. Use this when threads or goroutines",
			"are escalating. The daemon must be running (zqk scheduler start). Not supported",
			"on Windows (SIGUSR1 not available)."},
		Examples: []string{"Capture process dump from running daemon", "%s scheduler dump"},
	})
}

// NewSchedulerEventsAggregateCommandBuilder creates a new scheduler_aggregate command
func NewSchedulerEventsAggregateCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "aggregate",
		Short: "Aggregate diagnostics.jsonl into scheduler-metrics-summary.json",
		Description: []string{"Reads .zqk/scheduler/diagnostics.jsonl (JSONL), aggregates job execution events",
			"(completed/failed, duration), and writes .zqk/scheduler/scheduler-metrics-summary.json.",
			"Use this periodically (e.g. via a scheduler job) to keep a summary for performance",
			"analysis and reporting. See docs/archive/system_health/SCHEDULER_EVENTS_AND_METRICS.md."},
		Examples: []string{"Aggregate scheduler events into metrics summary", "%s scheduler events aggregate"},
	})
}

// NewSchedulerEventsCommandBuilder creates a new scheduler_events command
func NewSchedulerEventsCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "events",
		Short: "Scheduler events (health view over metrics summary)",
		Description: []string{"Commands for viewing scheduler events and health. The health subcommand shows",
			"a health view over scheduler-metrics-summary (failures and slow jobs). The",
			"summary is updated automatically by the SCH-evag timer job."},
	})
}

// NewSchedulerEventsHealthCommandBuilder creates a new scheduler_health command
func NewSchedulerEventsHealthCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:         "health",
		Short:       "health command",
		Description: []string{"health command"},
	})
}

// NewSchedulerHealthCheckCommandBuilder creates a new scheduler_health_check command
func NewSchedulerHealthCheckCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "health-check",
		Short: "Check scheduler daemon health (external health check)",
		Description: []string{"Check if the scheduler daemon is running and healthy.",
			"",
			"This command runs externally (not inside the scheduler daemon) and can be used",
			"by monitoring systems, cron jobs, or other external tools to detect if the",
			"scheduler daemon is down.",
			"",
			"Exit codes:",
			"  0 - Scheduler is healthy (running and responsive)",
			"  1 - Scheduler is unhealthy (down but should be running, or keep-alive is stale)",
			"  2 - Error checking scheduler status",
			"",
			"This command checks:",
			"  - PID file exists and process is running",
			"  - Keep-alive file is fresh (updated within last 2 minutes)",
			"  - If enabled timer jobs exist, daemon must be running"},
		Examples: []string{"Check scheduler health", "%s scheduler health-check"},
	})
}

// NewSchedulerHealthCommandBuilder creates a new scheduler_ command
func NewSchedulerHealthCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "",
		Short: "Generated spec for health",
	})
}

// NewSchedulerHistoryCommandBuilder creates a new scheduler_history command
func NewSchedulerHistoryCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "history",
		Short: "Show execution history summary for scheduler jobs",
		Description: []string{"Show a summary of scheduler job execution history including success/failure counts and last run time.",
			"",
			"This command queries audit events to show:",
			"  - Total runs per job",
			"  - Success count",
			"  - Failure count",
			"  - Last run time",
			"  - Last run outcome",
			"",
			"Performance tips:",
			"  - Use --limit to reduce the number of events queried (default: 5000)",
			"  - Use --since and --until to narrow the time range",
			"  - Use --job-id to filter by specific job"},
		Examples: []string{"Show history for all jobs", "%s scheduler history",
			"Show history for a specific job", "%s scheduler history --job-id SCH-001",
			"Show history since last week", "%s scheduler history --since 7d --bypass-cache"},
		Flags: func(builder *clipkg.CommandBuilder) {
			builder.AddStringFlag("job-id", "", "", "Filter by specific job ID (optional)")
			builder.AddIntFlag("limit", "", 5000, "Maximum number of audit events to query (default: 5000, only used with --bypass-cache or time filters)")
			builder.AddStringFlag("since", "", "", "Only show history since this time. Supports: RFC3339, Relative (1h, 12h, 1d, 7d, 30d), Natural (today, yesterday, this-week, last-week, this-month, last-month, this-year). Note: Time filters require --bypass-cache")
			builder.AddStringFlag("until", "", "", "Only show history until this time. Supports: RFC3339, Relative (1h, 12h, 1d, 7d, 30d), Natural (today, yesterday, now). Note: Time filters require --bypass-cache")
			builder.AddBoolFlag("bypass-cache", "", false, "Bypass cache and query audit events directly (slower, but more complete for small datasets)")
		},
	})
}

// NewSchedulerIssuesBundleHealthCommandBuilder creates a new scheduler_issues_bundle_health command
func NewSchedulerIssuesBundleHealthCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "issues-bundle-health",
		Short: "Cross-check issues.json against test-bundle health.jsonl",
		Description: []string{"When the scheduler is idle or issues.json still lists old SCH-run job failures, compare each",
			"entry to lines in .zqk/logs/scheduler/cvs/test-bundles/health.jsonl. For each bundle",
			"fingerprint, the last line in the scanned file wins — so a newer pass (possibly under a new",
			"job id) shows as green even if issues.json was never cleared.",
			"",
			"Use together with: zqk scheduler test-failures list, zqk scheduler test-failures health."},
		Examples: []string{"Compare issues.json to bundle health", "%s scheduler issues-bundle-health"},
		Flags: func(builder *clipkg.CommandBuilder) {
			builder.AddIntFlag("health-limit", "", 100000, "Max successfully parsed JSON rows from health.jsonl (full scan from start; error if exceeded)")
		},
	})
}

// NewSchedulerIssuesCommandBuilder creates a new scheduler_issues command
func NewSchedulerIssuesCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:         "issues",
		Short:       "issues command",
		Description: []string{"issues command"},
	})
}

// NewSchedulerListCommandBuilder creates a new scheduler_list command
func NewSchedulerListCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:         "list",
		Short:       "List all scheduler jobs",
		Description: []string{"List all scheduler_job objects with their status and schedule information."},
		Examples:    []string{"List all scheduler jobs", "%s scheduler list"},
	})
}

// NewSchedulerPrintCursorPasteApplescriptCommandBuilder creates a new scheduler_print_cursor_paste_applescript command
func NewSchedulerPrintCursorPasteApplescriptCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "print-cursor-paste-applescript",
		Short: "Print AppleScript for agent chat paste (same as --paste-cursor; honors ZQK_CURSOR_PASTE_PREFIX_STEPS)",
		Description: []string{"Prints the AppleScript used by `zqk scheduler convergence measure --format agent-prompt --paste-cursor`.",
			"Respects `ZQK_CURSOR_PASTE_PREFIX_STEPS`; default tokens: cmd_shift_e, escape, option_cmd_e, option_cmd_e.",
			"",
			"For shell: `zqk scheduler print-cursor-paste-applescript | osascript -`"},
		Examples: []string{"Pipe generated script to osascript", "%s scheduler print-cursor-paste-applescript | osascript -"},
	})
}

// NewSchedulerPrintIDEPasteApplescriptCommandBuilder creates a new scheduler_print_ide_paste_applescript command
func NewSchedulerPrintIDEPasteApplescriptCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "print-ide-paste-applescript",
		Short: "Print AppleScript for agent chat paste (same as --paste-ide; honors ZQK_CURSOR_PASTE_PREFIX_STEPS)",
		Description: []string{"Prints the AppleScript used by `zqk scheduler convergence measure --format agent-prompt --paste-ide`.",
			"Respects `ZQK_CURSOR_PASTE_PREFIX_STEPS`; default tokens: cmd_shift_e, escape, option_cmd_e, option_cmd_e.",
			"",
			"For shell: `zqk scheduler print-ide-paste-applescript | osascript -`"},
		Examples: []string{"Pipe generated script to osascript", "%s scheduler print-ide-paste-applescript | osascript -"},
	})
}

// NewSchedulerRecordCvsOrchestrateRunCommandBuilder creates a new scheduler_record_cvs_orchestrate_run command
func NewSchedulerRecordCvsOrchestrateRunCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "record-cvs-orchestrate-run",
		Short: "Append one validated cvs_orchestrate_run_v1 JSON line (stdin) to the orchestrate JSONL log",
		Description: []string{"Reads a single JSON object from stdin (one line; schema_version must be cvs_orchestrate_run_v1),",
			"validates it, and appends one compact line to .zqk/logs/scheduler/cvs_orchestrate_runs.jsonl",
			"unless --jsonl-path overrides the destination.",
			"",
			"This is the in-repo implementation of the durable observability event for",
			"scripts/cvs_convergence_orchestrate.sh: the shell builds the JSON (same fields as before) and",
			"pipes it here so validation and file append live in Go alongside rollup_status_core.jsonl-style",
			"append-only logs. observability.Recorder backends remain optional; the JSONL file is the",
			"contract for operators and tooling.",
			"",
			"See docs/architecture/CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md (Appendix D)."},
		Flags: func(builder *clipkg.CommandBuilder) {
			builder.AddStringFlag("jsonl-path", "", "", "Override JSONL path (default: .zqk/logs/scheduler/cvs_orchestrate_runs.jsonl under project root)")
			builder.AddBoolFlag("dry-run", "", false, "Validate stdin JSON only; do not append")
		},
	})
}

// NewSchedulerRerunCommandBuilder creates a new scheduler_ command
func NewSchedulerRerunCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "",
		Short: "Generated spec for rerun",
	})
}

// NewSchedulerScanTestsCommandBuilder creates a new scheduler_scan_tests [flags] command
func NewSchedulerScanTestsCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "scan-tests [flags]",
		Short: "Scan and schedule tests using the test scanner",
		Description: []string{"Scan Go test files and schedule them as scheduler jobs.",
			"",
			"The scanner:",
			"  - Identifies test functions in *_test.go files",
			"  - Detects parallel safety (t.Parallel())",
			"  - Bundles tests that can run safely in parallel",
			"  - Schedules tests based on runtime metrics",
			"  - Creates optimized scheduler jobs",
			"",
			"Modes:",
			"  --test <name>     - Scan and schedule a single test by name",
			"  --tests <names>   - Scan and schedule multiple tests (comma-separated)",
			"  --package <path>  - Scan and schedule all tests in one or more packages (comma-separated, e.g. ./pkg/a,./pkg/b)",
			"  --all             - Scan and schedule all tests in the project",
			"  --load-bundle <name> - Load a previously saved bundle",
			"  --load-bundles <names> - Load multiple bundles (comma-separated) for regression",
			"",
			"Bundle Management:",
			"  --save-bundle <name> - Save created bundles with this name",
			"  --load-bundle <name>  - Load a previously saved bundle",
			"  --load-bundles <names> - Load multiple saved bundles (comma-separated)",
			"  --list-bundles       - List all saved bundles",
			"  --show-bundle        - Show bundle details without scheduling",
			"  --only <indexes>     - Run only specified test indexes (e.g., 0,2,5)",
			"  --skip <indexes>     - Skip specified test indexes (e.g., 1,3)"},
		Examples: []string{"Schedule a single test", "%s scheduler scan-tests --test TestMyFeature",
			"Schedule all tests in a package", "%s scheduler scan-tests --package ./pkg/storage",
			"Schedule all tests in multiple packages (comma-separated)", "%s scheduler scan-tests --package ./pkg/paths,./pkg/agentdelivery",
			"Schedule all tests in project", "%s scheduler scan-tests --all",
			"Run regression: schedule multiple saved bundles", "%s scheduler scan-tests --load-bundles pkg-storage-1,pkg-scheduler-2,cmd-zqk-system-2"},
		Flags: func(builder *clipkg.CommandBuilder) {
			builder.AddStringFlag("test", "", "", "Single test name to schedule")
			builder.AddStringFlag("tests", "", "", "Comma-separated list of test names to schedule")
			builder.AddStringFlag("package", "", "", "One package path or comma-separated paths (e.g., ./pkg/storage or ./pkg/a,./pkg/b)")
			builder.AddBoolFlag("all", "", false, "Schedule all tests in the project")
			builder.AddIntFlag("max-bundle-size", "", 10, "Maximum number of tests per bundle")
			builder.AddIntFlag("max-parallel", "", 4, "Maximum number of parallel bundles")
			builder.AddStringFlag("save-bundle", "", "", "Save the created bundles with this name")
			builder.AddStringFlag("load-bundle", "", "", "Load a previously saved bundle")
			builder.AddStringFlag("load-bundles", "", "", "Load multiple saved bundles (comma-separated names); schedules all for regression runs")
			builder.AddStringFlag("only", "", "", "Comma-separated list of test indexes to run (e.g., 0,2,5)")
			builder.AddStringFlag("skip", "", "", "Comma-separated list of test indexes to skip (e.g., 1,3)")
			builder.AddBoolFlag("list-bundles", "", false, "List all saved bundles")
			builder.AddBoolFlag("show-bundle", "", false, "Show bundle details without scheduling")
			builder.AddBoolFlag("overwrite", "", false, "Overwrite existing bundle when saving")
			builder.AddBoolFlag("merge", "", false, "Merge new tests with existing bundle (adds new, keeps existing)")
			builder.AddBoolFlag("remove-missing", "", false, "Remove tests from bundle that no longer exist (use with --merge)")
			builder.AddBoolFlag("setup-bundles", "", false, "Set up initial bundles for the project (scans all tests and creates bundles)")
			builder.AddBoolFlag("delete-all-bundles", "", false, "Delete all saved test bundles from .zqk/test-bundles (does not remove scheduler_job objects like SCH-run-bundle-*; use before --setup-bundles to recreate)")
			builder.AddBoolFlag("suggest-timeout", "", false, "Print suggested timeout in seconds for --package (dynamic from timing data and package defaults); if multiple comma-separated packages, prints the maximum; exit without scheduling")
			builder.AddStringFlag("source-root", "", "", "Go module root for go test cwd (Local CI workdir). Defaults to .zqk/local-ci/workdir when present unless --live-source. Logs and SCH-run objects stay on studio project root.")
			builder.AddBoolFlag("live-source", "", false, "Force go test against the live studio tree (not Local CI). Escape hatch; not CI-honest.")
		},
	})
}

// NewSchedulerServiceCommandBuilder creates a new scheduler_service command
func NewSchedulerServiceCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "service",
		Short: "Manage per-root host OS scheduler units (launchd/systemd)",
		Description: []string{"Install, list, start, stop, rebind, and GC host OS supervisor units for the",
			"scheduler daemon — one unit per project root. Replaces ensure-cron prosthetics.",
			"See docs/architecture/SCHEDULER_HOST_SERVICE_AND_CLUSTER_STATUS.md."},
		Examples: []string{"Install a LaunchAgent/systemd unit for this project root", "%s scheduler service install --root .",
			"List registered units", "%s scheduler service list",
			"Garbage-collect units whose roots are gone", "%s scheduler service gc"},
	})
}

// NewSchedulerServiceGcCommandBuilder creates a new scheduler_gc command
func NewSchedulerServiceGcCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:      "gc",
		Short:    "Remove host units whose project roots are gone or desired_state=absent",
		Examples: []string{"Garbage-collect orphans", "%s scheduler service gc"},
	})
}

// NewSchedulerServiceInstallCommandBuilder creates a new scheduler_install command
func NewSchedulerServiceInstallCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "install",
		Short: "Install a per-root host OS unit for the scheduler daemon",
		Description: []string{"Writes a launchd LaunchAgent (macOS) or systemd --user unit (Linux), updates the",
			"host registry, and enables KeepAlive/Restart. Prefers workshop stable",
			"(.zqk/bin/zqk-stable) when present so host units match MCP/scheduler daemons;",
			"falls back to tip bin/zqk when stable is absent."},
		Examples: []string{"Install for current directory", "%s scheduler service install --root ."},
		Flags: func(builder *clipkg.CommandBuilder) {
			builder.AddStringFlag("root", "", ".", "Project root to supervise")
		},
	})
}

// NewSchedulerServiceListCommandBuilder creates a new scheduler_list command
func NewSchedulerServiceListCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:         "list",
		Short:       "List registered scheduler host service units",
		Description: []string{"Lists host registry entries. Use --orphans to show units whose abs_root is missing."},
		Examples:    []string{"List all", "%s scheduler service list"},
		Flags: func(builder *clipkg.CommandBuilder) {
			builder.AddBoolFlag("orphans", "", false, "Only show entries whose project root directory is missing")
		},
	})
}

// NewSchedulerServiceRebindCommandBuilder creates a new scheduler_rebind command
func NewSchedulerServiceRebindCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:      "rebind",
		Short:    "Update abs_root for an existing root_id (path rename/move)",
		Examples: []string{"Rebind after directory move", "%s scheduler service rebind --root-id <id> --new-path /new/path"},
		Flags: func(builder *clipkg.CommandBuilder) {
			builder.AddStringFlag("root-id", "", "", "Stable root_id from the host registry")
			builder.AddStringFlag("new-path", "", "", "New absolute or relative project path")
		},
	})
}

// NewSchedulerServiceStartCommandBuilder creates a new scheduler_start command
func NewSchedulerServiceStartCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:      "start",
		Short:    "Start a registered scheduler host unit",
		Examples: []string{"Start by path", "%s scheduler service start --root ."},
		Flags: func(builder *clipkg.CommandBuilder) {
			builder.AddStringFlag("root", "", "", "Root id or absolute/relative project path")
		},
	})
}

// NewSchedulerServiceStatusCommandBuilder creates a new scheduler_status command
func NewSchedulerServiceStatusCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:      "status",
		Short:    "Show OS unit status for a registered root",
		Examples: []string{"Status", "%s scheduler service status --root ."},
		Flags: func(builder *clipkg.CommandBuilder) {
			builder.AddStringFlag("root", "", "", "Root id or project path")
		},
	})
}

// NewSchedulerServiceStopCommandBuilder creates a new scheduler_stop command
func NewSchedulerServiceStopCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:      "stop",
		Short:    "Stop a registered scheduler host unit",
		Examples: []string{"Stop by path", "%s scheduler service stop --root ."},
		Flags: func(builder *clipkg.CommandBuilder) {
			builder.AddStringFlag("root", "", "", "Root id or project path")
		},
	})
}

// NewSchedulerServiceUninstallCommandBuilder creates a new scheduler_uninstall command
func NewSchedulerServiceUninstallCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:      "uninstall",
		Short:    "Uninstall a registered scheduler host unit (leaves project data)",
		Examples: []string{"Uninstall", "%s scheduler service uninstall --root ."},
		Flags: func(builder *clipkg.CommandBuilder) {
			builder.AddStringFlag("root", "", "", "Root id or project path")
		},
	})
}

// NewSchedulerSkipWindowClearCommandBuilder creates a new scheduler_clear command
func NewSchedulerSkipWindowClearCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "clear",
		Short: "Remove skip_window.yaml",
		Description: []string{"Deletes `.zqk/scheduler/skip_window.yaml` so policy evaluation no longer applies",
			"bulk skip-window rules."},
	})
}

// NewSchedulerSkipWindowCommandBuilder creates a new scheduler_skip_window command
func NewSchedulerSkipWindowCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "skip-window",
		Short: "Set or clear scheduler skip window (bulk skip matching jobs until a time)",
		Description: []string{"Manage `.zqk/scheduler/skip_window.yaml`. When active, the scheduler policy engine skips",
			"matching job executions until `until` (handlers do not run). Use during instability or when",
			"you want timer-driven jobs to stay quiet until the workspace is healthy again.",
			"",
			"Subcommands: set, clear, status."},
		Examples: []string{"Skip convergence_session_tick timer jobs for 2 hours", "%s scheduler skip-window set --for 2h --job-type convergence_session_tick --trigger-type timer",
			"Skip all jobs for 30 minutes (use with care)", "%s scheduler skip-window set --for 30m --match-all",
			"Show active window", "%s scheduler skip-window status",
			"Remove skip window", "%s scheduler skip-window clear"},
	})
}

// NewSchedulerSkipWindowSetCommandBuilder creates a new scheduler_set command
func NewSchedulerSkipWindowSetCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "set",
		Short: "Write skip_window.yaml (matching jobs skip until until)",
		Description: []string{"Writes `.zqk/scheduler/skip_window.yaml`. Requires `--for` or `--until`, and",
			"`--match-all` and/or `--job-type` / `--trigger-type` / `--title-contains` filters."},
		Examples: []string{"Two-hour window for convergence tick jobs", "%s scheduler skip-window set --for 2h --job-type convergence_session_tick --trigger-type timer"},
		Flags: func(builder *clipkg.CommandBuilder) {
			builder.AddStringFlag("for", "", "", "Duration from now (e.g. 2h, 30m); mutually exclusive with --until")
			builder.AddStringFlag("until", "", "", "Absolute end time (RFC3339); mutually exclusive with --for")
			builder.AddBoolFlag("match-all", "", false, "Skip all jobs until until (use with care)")
			builder.AddStringArrayFlag("job-type", "", "Repeat for each job_type to match (e.g. convergence_session_tick)")
			builder.AddStringArrayFlag("trigger-type", "", "Repeat for each trigger_type to match (e.g. timer)")
			builder.AddStringFlag("title-contains", "", "", "Substring match on job title (case-insensitive)")
		},
	})
}

// NewSchedulerSkipWindowStatusCommandBuilder creates a new scheduler_status command
func NewSchedulerSkipWindowStatusCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "status",
		Short: "Show active skip window if any",
		Description: []string{"Prints the active skip window (until time and filters) when the file exists and",
			"`until` is still in the future."},
	})
}

// NewSchedulerStartCommandBuilder creates a new scheduler_start command
func NewSchedulerStartCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "start",
		Short: "Start the scheduler daemon",
		Description: []string{"Start the scheduler daemon to execute scheduled jobs.",
			"",
			"The daemon will:",
			"  - Load all enabled scheduler_job objects",
			"  - Schedule timer-based jobs according to their cron expressions",
			"  - Listen for event and lifecycle triggers",
			"  - Execute jobs with timeout and retry logic",
			"  - Create audit events for job execution",
			"",
			"Default: runs in the background (detached process). Use --foreground to attach and stream logs.",
			"If the daemon exits (crash or stop), run zqk scheduler start again to bring it back."},
		Examples: []string{"Start scheduler daemon", "%s scheduler start"},
	})
}

// NewSchedulerStateCommandBuilder creates a new scheduler_state command
func NewSchedulerStateCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "state",
		Short: "Summarize scheduler state-file health",
		Description: []string{"Summarize scheduler execution state files under .zqk/scheduler/state.",
			"Shows counts by state and highlights stale in_progress/deferred entries."},
		Examples: []string{"Show state summary with default stale threshold", "%s scheduler state",
			"Use a 2 hour stale threshold", "%s scheduler state --stale-after 2h",
			"Move legacy flat state YAML into per-job directories (one-time cleanup)", "%s scheduler state --migrate"},
		Flags: func(builder *clipkg.CommandBuilder) {
			builder.AddDurationFlag("stale-after", "", "30m", "Mark in_progress/deferred entries older than this as stale")
			builder.AddBoolFlag("migrate", "", false, "Move legacy flat *.yaml state files into per-job subdirectories under .zqk/scheduler/state/")
		},
	})
}

// NewSchedulerStatusCommandBuilder creates a new scheduler_status command
func NewSchedulerStatusCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:         "status",
		Short:       "Show scheduler daemon status",
		Description: []string{"Show the current status of the scheduler daemon and running jobs."},
		Examples:    []string{"Show scheduler status", "%s scheduler status"},
	})
}

// NewSchedulerStopCommandBuilder creates a new scheduler_stop command
func NewSchedulerStopCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "stop",
		Short: "Stop the scheduler daemon",
		Description: []string{"Stop the running scheduler daemon.",
			"",
			"This gracefully stops the scheduler, allowing current jobs to complete",
			"before shutting down."},
		Examples: []string{"Stop scheduler daemon", "%s scheduler stop"},
		Flags: func(builder *clipkg.CommandBuilder) {
			builder.AddBoolFlag("wait", "", false, "Wait for the daemon process to exit")
			builder.AddDurationFlag("max-wait", "", "3m", "Max time to wait for exit when --wait is set (matches daemon clean-shutdown budget)")
			builder.AddBoolFlag("force", "", false, "Force kill the daemon (SIGKILL). Use when graceful stop is stuck.")
		},
	})
}

// NewSchedulerSubmitCommandBuilder creates a new scheduler_submit <command> [args...] command
func NewSchedulerSubmitCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "submit <command> [args...]",
		Short: "Submit a command as a background job to the scheduler",
		Description: []string{"Submit a command to run as a background job in the scheduler.",
			"",
			"This creates a one-time scheduler job that executes immediately. The job runs",
			"in the background and you can check its status using 'scheduler activity'",
			"or 'scheduler history'.",
			"",
			"The command is executed using run_wrapper, which provides:",
			"  - Progress tracking (stdout/stderr capture)",
			"  - Timeout management",
			"  - Retry logic",
			"  - Execution metrics"},
		Flags: func(builder *clipkg.CommandBuilder) {
			builder.AddIntFlag("max-runtime", "", 3600, "Maximum runtime in seconds (0 = no timeout, default: 3600)")
			builder.AddStringFlag("workdir", "", "", "Working directory for command execution")
			builder.AddStringArrayFlag("env", "", "Environment variables (format: KEY=VALUE, can be specified multiple times)")
			builder.AddIntFlag("retry", "", 0, "Number of retry attempts on failure (default: 0)")
			builder.AddIntFlag("retry-delay", "", 5, "Delay between retries in seconds (default: 5)")
			builder.AddStringFlag("title", "", "", "Job title (default: auto-generated from command)")
			builder.AddStringFlag("callback-completion", "", "", "Command to run on job completion (receives JSON payload via stdin)")
			builder.AddStringFlag("callback-failure", "", "", "Command to run on job failure (receives JSON payload via stdin)")
		},
	})
}

// NewSchedulerTestCommandBuilder creates a new scheduler_ command
func NewSchedulerTestCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "",
		Short: "Generated spec for test",
	})
}

// NewSchedulerTestFailuresCommandBuilder creates a new scheduler_test_failures command
func NewSchedulerTestFailuresCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "test-failures",
		Short: "List and manage failing tests",
		Description: []string{"List and manage failing tests from recent test runs.",
			"",
			"Data sources:",
			"- Callback logs under .zqk/callbacks (when callbacks are configured)",
			"- Scheduler test-bundle shared events: .zqk/logs/scheduler/cvs/test-bundles/events.jsonl",
			"- Rolling health timeline: .zqk/logs/scheduler/cvs/test-bundles/health.jsonl",
			"",
			"Failed bundle event lines include test_failures and suggested_rerun_commands (copy-paste go test",
			"lines). Subcommands list and rerun consider both callbacks and test-bundle events.",
			"",
			"Convergence measurement (health rollup, CVS handoff) lives under `zqk scheduler convergence`",
			"(measure, overseer), not under test-failures."},
		Examples: []string{"List failing tests (callbacks + test-bundle events)", "%s scheduler test-failures list",
			"List failures for a specific package", "%s scheduler test-failures list --package ./pkg/storage",
			"Re-run only failing tests (creates scheduler jobs)", "%s scheduler test-failures rerun",
			"Summarize test-bundle health from health.jsonl", "%s scheduler test-failures health",
			"Convergence measure for automation (JSON)", "%s scheduler convergence measure --format json",
			"Convergence JSON without literal gate scripts (faster; full gates + matrix in cvs_outcome_rollup.py)", "%s scheduler convergence measure --format json --skip-rollup-gates",
			"Convergence measure with suggested CVS fields for object update", "%s scheduler convergence measure --format json --session-id CVS-001",
			"Convergence with phase router alignment (current CVS phase + flow variant)", "%s scheduler convergence measure --format json --session-id CVS-001 --current-phase c5_verify --flow-variant scheduler_fast",
			"Object update payload only (pipe to file for zqk object update --file)", "%s scheduler convergence measure --format json --session-id CVS-001 | jq '.suggested_convergence_session_fields.object_update_body'",
			"Include iteration tombstone (before_state_snapshot) in object update payload", "%s scheduler convergence measure --format json --session-id CVS-001 --stamp-tombstone | jq '.suggested_convergence_session_fields.object_update_body'",
			"Finalize: merge predictions with retrospective debrief into object update payload", "%s scheduler convergence measure --format json --session-id CVS-001 --finalize-debrief | jq '.suggested_convergence_session_fields.object_update_body.predictions'",
			"Handoff: operator debrief notes on the CVS for the next convergence session", "%s scheduler convergence measure --format json --session-id CVS-001 --debrief-notes 'Next: focus pkg/scheduler first; watch retention bundle timeouts.' | jq '.suggested_convergence_session_fields.object_update_body.debrief_notes'",
			"Agent prompt (markdown for chat) — same command with --format agent-prompt", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001",
			"Agent prompt to file only (no CVS write; omit --persist-session)", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001 -o .zqk/logs/drift/cvs_agent_prompt_latest.md",
			"Agent prompt copied to clipboard (macOS pbcopy)", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001 --copy",
			"Agent prompt then AppleScript: activate Cursor, ⌘Y, ⌘V, Return (macOS; Accessibility)", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001 --paste-cursor",
			"Agent prompt in test / non-directive mode (banner + machine-parseable comment for queue integration)", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001 --attention-mode test_non_directive",
			"Persist CVS + log file + clipboard for chat (automated loop handoff; macOS)", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001 --persist-session -o .zqk/logs/drift/cvs_agent_prompt_latest.md --copy",
			"Persist CVS measurement then emit agent prompt only (no clipboard; use --copy or --paste-cursor for chat)", "%s scheduler convergence measure --format agent-prompt --session-id CVS-001 --persist-session",
			"Coordinator overseer: rollup + nested CVS tree + arbitrated prompt line", "%s scheduler convergence overseer --coordinator-session-id CVS-001 --format json",
			"Analyze failure patterns (optional backlog items)", "%s scheduler test-failures analyze"},
	})
}

// NewSchedulerTriggerCommandBuilder creates a new scheduler_trigger <job_id> command
func NewSchedulerTriggerCommandBuilder() *cobra.Command {
	return buildSchedulerCommand(SchedulerCommandConfig{
		Use:   "trigger <job-id>",
		Short: "Manually trigger a scheduler job",
		Description: []string{"Manually trigger execution of a scheduler job by ID.",
			"",
			"This allows you to run a job immediately, regardless of its trigger_type.",
			"The job must be enabled and support manual triggering.",
			"",
			"Use --pre-commit for pre-commit category jobs (lint, policy, integrity). When set,",
			"the job's callback_on_completion runs after the job finishes (writes pre-commit",
			"category and runs aggregate) so the hook sees results. Timer runs do not run callbacks."},
		Examples: []string{"Trigger a job immediately", "%s scheduler trigger SCH-001",
			"Trigger pre-commit lint job so callback runs and hook sees result", "%s scheduler trigger SCH-pre-commit-lint --pre-commit"},
		Flags: func(builder *clipkg.CommandBuilder) {
			builder.AddBoolFlag("pre-commit", "", false, "Run with pre-commit origin so the job's callback_on_completion runs (writes pre-commit category and aggregate)")
		},
	})
}
