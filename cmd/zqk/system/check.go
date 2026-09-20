package system

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/systemcheckwake"
)

// checkCmd represents the check command
var checkCmd *cobra.Command

func NewCheckCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Check object health: registration, lifecycle, and policy compliance",
		"Check object health and compliance.",
		"",
		"Validates that an object:",
		"  - Is properly registered with the system",
		"  - Adheres to its lifecycle definition",
		"  - Complies with policies for its object kind",
		"  - Has valid references (default; --fast skips refs and is NOT an authoritative health verdict)",
		"  - Has file integrity verified using hash indexes by kind/type",
		"",
		"Results are organized into a 4-Layer Compliance Cake:",
		"  - Layer 0: CAS and Registry Integrity Blockers (Registry, Hash Mismatches, Parse Errors)",
		"  - Layer 1: Objects Needing Fixes (Business Preconditions, missing attributes, broken relations)",
		"  - Layer 2: Objects Ready for Promotion/Execution (Clean objects with 0 validation issues)",
		"  - Layer 3: Completed & Validated Items (Terminal objects passing all validation gates)",
		"",
		"--fast / --check-refs=false is a partial, read-only surface (skips reference integrity).",
		"Combined with --auto-fix or --force, --fast is dropped and refs are enabled so mutation",
		"uses the full surface. A green partial result does not mean the kernel is healthy.",
		"Autofix demotes only process_failure findings to status=error (illegal lifecycle /",
		"status preconditions), never data_completeness or employment_fitness; identity/",
		"governance kinds never autofix-demote.",
		"",
		"Usage patterns:",
		"  - For full-project checks with auto-fix, run with an explicit long timeout",
		"    (e.g., --timeout 3600s) so the run can complete and write results.",
		"  - Prefer background mode (--background) when running from CI or tools;",
		"    progress and completion are reported via the coordinator and logs.",
		"  - Avoid short timeouts (e.g., 30s, 60s) for 'check all --auto-fix' – they",
		"    almost always time out during CAS cleanup and validation.",
	).
		AddExample("Check a specific object (by ID)", "%s system check BLI-626").
		AddExample("Check all objects of a kind", "%s system check backlog_item").
		AddExample("Check all objects in the system", "%s system check all").
		AddExample("Check with verbose output", "%s system check BLI-626 --verbose").
		AddExample("Check with JSON output", "%s system check all --format json").
		AddExample("Partial check (refs skipped; not authoritative)", "%s system check all --fast").
		AddExample("Fast iteration: write failing IDs then re-check only those", "%s system check all --write-failing-ids tier1.txt --fast; %s system check --ids-from-file tier1.txt --fast").
		AddExample("Run in background (return immediately; completion via coordinator)", "%s system check all --auto-fix --background").
		AddExample("Full-project auto-fix with long timeout", "%s system check all --auto-fix --timeout 3600s --format json -o .zqk/logs/system-check.json").
		AddExample("Wake primary agent if draft/error/blocking/warning/info thresholds trip", "%s system check all --notify").
		AddExample("Wake a specific seat when thresholds trip", "%s system check all --notify <seat-agent-id>").
		ExcludeCommonFlags()

	// Use local cmd so parallel tests (t.Parallel()) each build their own command;
	// assigning to package-level checkCmd only at the end avoids "flag redefined" races.
	var c cobra.Command
	c.Use = "check [kind] [id...]"
	c.Args = cobra.MinimumNArgs(0)
	cmd := &c

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	// Add common flags
	cli.AddCommonFlags(cmd)

	// Add check-specific flags
	cmd.Flags().Bool("auto-fix", false, "Automatically fix recoverable issues (missing hashes and hash mismatches)")
	cmd.Flags().Bool("force", false, "Force fix hash mismatches (requires explicit confirmation, creates audit event)")
	cmd.Flags().Bool("auto-fix-scheduler", true, "Use scheduler for auto-fix batching (provides status updates and timeout handling, default: true for batches >= 10 issues)")
	cmd.Flags().Bool("check-refs", true, "Check reference integrity (enabled by default; --fast disables). Combined with --auto-fix/--force, refs are re-enabled")
	cmd.Flags().Bool("fast", false, "Partial check: skip reference integrity only. Not an authoritative health verdict. Combined with --auto-fix/--force, --fast is dropped")
	cmd.Flags().Int("tier", 0, "Filter results to only show objects with issues of the specified tier (1=blocking, 2=warning, 3=informational, 4=recommendation, 0=all)")
	cmd.Flags().Int("layer", -1, "Filter results to only show objects in the specified compliance layer (0=integrity, 1=needs_fixes, 2=ready, 3=complete, -1=all)")
	cmd.Flags().String("surface", "", "Filter issues by fitness surface (auth, agent_dispatch, system_check_l0, system_check_l1, admin_form, whats_next, list_default); annotates issue_class")
	cmd.Flags().String("delimiter", "\\n", "Delimiter to use between object IDs when using the 'ids' format (default: newline)")
	cmd.Flags().Bool("include-ids", false, "Include individual object IDs in status breakdown tables")
	cmd.Flags().Bool("details", false, "Show detailed list of object IDs for ready, completed, or static configurations (alias for --include-ids)")
	cmd.Flags().Bool("refresh-cache", false, "Rebuild object-id cache and validation-related caches; honored with --fast (fast still skips refs only). Also auto-applied when a significant CAS/cache change marker is present")
	cmd.Flags().Bool("clean-cache", false, "Clean stale entries from object ID cache (removes entries for deleted files)")
	cmd.Flags().Bool("clear-cache", false, "Break-glass: wipe validation_cache.json. Checker, binary, and lifecycle fingerprints normally invalidate on the next check; reserve this for admin recovery")
	cmd.Flags().Bool("validate-specs", false, "Validate object specifications: check that all fields have complete checklists with criteria traceability")
	cmd.Flags().String("cpu-profile", "", "Write CPU profile to file (for performance analysis)")
	cmd.Flags().String("goroutine-profile", "", "Write goroutine profile to file (for hang/deadlock analysis)")
	cmd.Flags().Bool("background", false, "Run in background: return immediately with operation ID; check continues and completion is reported via coordinator (default: wait and show progress)")
	cmd.Flags().String("metrics-file", "", "Write performance metrics to JSON file")
	cmd.Flags().Int("workers", 0, "Number of worker goroutines for async validation (0 = auto-detect based on CPU cores, default: NumCPU * 2, max: 32)")
	cmd.Flags().String("snapshot", "", "Save check results as snapshot to test-scenario folder (e.g., 'test-scenarios/check-violations')")
	cmd.Flags().String("ids-from-file", "", "Read object IDs from file (one per line, # ignored) and validate only those objects (fast re-check after fixes)")
	cmd.Flags().String("write-failing-ids", "", "After check, write object IDs with Tier 1 issues to this file (use with --ids-from-file for fast iteration)")
	cmd.Flags().Bool("unresolvable-only", false, "Filter output to only show unresolvable violations")
	cmd.Flags().Bool("cache-only", false, "Show cached validation results only (do not run validation, trigger background scan)")
	cmd.Flags().String("dispatch-to", "", "Dispatch health violations/issues as inbox items to the specified agent/role (e.g. 'QA-Engineer', 'Security-Engineer', 'technical-program-manager', or 'auto')")
	// Opt-in mesh wake when draft/error/blocking/warning/info thresholds trip (not automatic).
	// Bare --notify → primary orchestrator agent_id; --notify <id> overrides.
	cmd.Flags().String("notify", "", "Wake this agent seat if draft-plane / error-status / blocking / warning / informational thresholds trip after the check; omit value to use primary orchestrator (see .zqk/config/primary_orchestrator.json)")
	if f := cmd.Flags().Lookup("notify"); f != nil {
		f.NoOptDefVal = systemcheckwake.PrimarySentinel
	}

	cli.BindAsyncProgress(cmd, runCheck)
	checkCmd = cmd
	return cmd
}

// runCheck is implemented in check_impl.go
