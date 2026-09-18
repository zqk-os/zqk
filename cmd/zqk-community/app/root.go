package app

import (
	"context"
	"errors"
	"fmt"
	"os" // Added back for os.Executable and os.Exit
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/cmd/zqk/docman"
	"github.com/zqk-os/zqk/cmd/zqk/grep"

	commscheduler "github.com/zqk-os/zqk/cmd/zqk-community/scheduler"
	commsystem "github.com/zqk-os/zqk/cmd/zqk-community/system"
	commtest "github.com/zqk-os/zqk/cmd/zqk-community/test"
	"github.com/zqk-os/zqk/cmd/zqk/mcp"
	newcmd "github.com/zqk-os/zqk/cmd/zqk/new"
	"github.com/zqk-os/zqk/cmd/zqk/object"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/cmd/zqk/system"
	"github.com/zqk-os/zqk/cmd/zqk/tray"
	"github.com/zqk-os/zqk/cmd/zqk/utility"
	"github.com/zqk-os/zqk/cmd/zqk/workflow"
	"github.com/zqk-os/zqk/internal/cli"
	clicontext "github.com/zqk-os/zqk/internal/cli/context"
	"github.com/zqk-os/zqk/pkg/brand"
	clitool "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/diagnostics"
	"github.com/zqk-os/zqk/pkg/dispatch"
	"github.com/zqk-os/zqk/pkg/entitlements"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/federation"
	"github.com/zqk-os/zqk/pkg/federation/meshbroker"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/maintenance"
	"github.com/zqk-os/zqk/pkg/migration/detector"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/policyinterrupt"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/when"
	internal "github.com/zqk-os/zqk/pkg/zqkcli"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const EmptyValue = ""

// Version information - set at build time via ldflags
var (
	version   = "dev"
	buildDate = "unknown"
	gitCommit = "unknown"
)

// cpuProfileFile holds the open CPU profile output so PersistentPostRunE can stop and close it
var (
	cpuProfileFile       *fileutil.File
	goroutineProfilePath string
)

// writeGoroutineProfile writes a goroutine profile to file (for hang/deadlock analysis)
func writeGoroutineProfile(filename string) error {
	if filename == "" {
		return nil
	}
	f, err := fileutil.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	profile := pprof.Lookup("goroutine")
	if profile == nil {
		return fmt.Errorf("goroutine profile not available")
	}
	return profile.WriteTo(f, 0)
}

// parentDeathWatcherOnce ensures the parent-death watcher is started at most once per process.
var parentDeathWatcherOnce sync.Once

func init() {
	// Run PersistentPreRunE / PersistentPostRunE for the full ancestor chain (root → … → leaf)
	// instead of only the first parent that defines a hook. Otherwise nested subcommands
	// (e.g. `object <kind> fields`, `system data-cells`) never see the zqk root pre-run, the
	// object/system/internal group pre-runs, or declarative kind validation in the right order.
	cobra.EnableTraverseRunHooks = true

}

// startParentDeathWatcherIfSet starts a goroutine that exits this process when the parent (MCP server) dies.
// Used when ZQK_PARENT_PID is set (we were spawned by the MCP server). Platform-independent: polls
// os.Getppid(); when it no longer matches the expected parent PID (e.g. parent died and we were adopted by init),
// we exit so we don't become an orphaned tool process.
func startParentDeathWatcherIfSet() {
	envPID := zqkenv.ParentPID().Get()
	if envPID == EmptyValue {
		return
	}
	parentPID, err := strconv.Atoi(envPID)
	if err != nil || parentPID <= 0 {
		return
	}
	goroutinelabels.NewGoroutine("app", "parent death watcher").StartSimple(func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if os.Getppid() != parentPID {
				os.Exit(0)
			}
		}
	})
}

// mcpInitTrace logs a single line when running as an MCP tool subprocess (ZQK_MCP_ACCOUNT_ID set).
// Used to diagnose slow CLI init: each line has elapsed seconds and a label (e.g. "storage_ready").
// MCP server captures stderr; on timeout the trace shows where init hung.
// POL-CODE-007: route through logging framework (system profile writes to stderr).
func mcpInitTrace(start time.Time, label string) {
	if zqkenv.MCPAccountID().Get() == EmptyValue {
		return
	}
	elapsed := time.Since(start).Seconds()
	logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
		Debug(fmt.Sprintf("[MCP_INIT_TRACE] %.2fs %s", elapsed, label)).
		Log()
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = func() *cobra.Command {
	c := cobra.Command{
		Use:     paths.CLICommandName,
		Version: version,
	}
	return &c
}()

// registerCommandsOnce ensures command registration (including dynamic kind subcommands)
// runs once per process. Registration is deferred so tests can set ZQK_TEST_ROOT before
// calling NewRootCommand() and get correct kind routing (e.g. object backlog_item fields).
var rootCmdMu sync.Mutex
var registerCommandsOnce sync.Once

func ensureCommandsRegistered() {
	rootCmdMu.Lock()
	defer rootCmdMu.Unlock()
	ensureCommandsRegisteredLocked()
}

func ensureCommandsRegisteredLocked() {
	registerCommandsOnce.Do(func() {
		registerCommands()
		cli.ApplyBrandingToCommandTree(rootCmd, brand.ExecutableName(), brand.ProductName())
		cli.RegisterQuarantinedCommands(rootCmd)
	})
	// Re-run kind registration every time so tests that set ZQK_TEST_ROOT after first
	// NewRootCommand() still get dynamic kind subcommands (e.g. object backlog_item fields).
	objCmd, _, _ := rootCmd.Find([]string{"object"})
	if objCmd != nil && objCmd.Name() == "object" {
		object.RegisterDynamicKindCommands(objCmd)
	}
	internalCmd, _, _ := rootCmd.Find([]string{"internal"})
	if internalCmd != nil && internalCmd.Name() == "internal" {
		internal.RegisterDynamicInternalKindCommands(internalCmd)
	}
}

// NewRootCommand returns the fully configured root command.
// This is intended for in-process callers (tests, tools) that want to
// execute CLI flows without going through main()/Execute().
// NOTE: This returns the global rootCmd instance; tests should be careful
// about cross-test mutation of flags and subcommands.
func NewRootCommand() *cobra.Command {
	// Inject entitlement checker to cli
	clitool.CheckEntitlementFunc = entitlements.CheckEntitlementBundle
	ensureCommandsRegistered()
	return rootCmd
}

func init() {
	zqkenv.IsCommunityEdition = true

	// Build help for root command
	helpBuilder := clitool.NewHelpBuilder().
		WithShort("Knowledge kernel for first-run project work").
		WithDescriptionLines(
			"First-run CLI for a local knowledge kernel: objects, plans, docs, MCP, and a thin scheduler.",
			"This SKU does not ship an admin codegen binary, mesh join, or studio origination verbs.",
		).
		AddSection("Command surfaces",
			"Everyday: object, system, test, workflow, new, grep, quickstart.\n"+
				"Integrations: mcp, scheduler.\n"+
				"Docs: docman. Shortcuts: tray.\n"+
				"This binary does not ship admin codegen, monitor registry, or keystore.",
		).
		WithAutoDiscoverSubcommands(true)
	helpBuilder.ApplyToCommand(rootCmd)

	// Wire synonym resolver and cache handler when storage is first created (on demand by object commands).
	// Warm synonym and kind mapper caches so first List does not pay full init (see CPU_PROFILE_OBJECT_LIST_ANALYSIS.md).
	cli.OnStorageCreated = func(provider storage.ObjectStorageProvider) {
		resolver := objects.GetGlobalSynonymResolver()
		if resolver != nil {
			resolver.SetSynonymLoader(storage.NewStorageSynonymLoader(provider))
			_ = resolver.Initialize() // load synonyms once so GetCanonicalKind/GetDirectoryFromKind are fast
		}
		if fileStorage, ok := provider.(*storage.FileObjectStorage); ok && fileStorage != nil {
			if projectRoot := fileStorage.GetProjectRoot(); projectRoot != EmptyValue {
				objects.PrewarmGlobalsForProjectRoot(projectRoot)
			}
			// Eager-init bucket strategy registry so first Create does not pay (OBJECT_OPERATIONS_PERFORMANCE.md)
			fileStorage.EnsureBucketStrategyRegistryReady(context.Background())
		}
		storage.SetCacheOperationHandler(func(cacheCtx *pkgctx.CacheContext) error {
			projectRoot := cli.ResolveProjectRoot(".")
			var err error
			switch cacheCtx.Operation {
			case pkgctx.CacheOperationUpdate:
				err = system.UpdateObjectIDCache(cacheCtx.NewID, cacheCtx.Kind, cacheCtx.FilePath)
				if err == nil {
					storage.InvalidateListCacheForKind(cacheCtx.Kind)
					storage.ClearObjectIDCachePending(projectRoot, cacheCtx.NewID)
				}
				system.InvalidateValidationCacheForCacheContext(projectRoot, cacheCtx)
				return err
			case pkgctx.CacheOperationInvalidate:
				system.InvalidateObjectIDCache(cacheCtx.OldID)
				storage.InvalidateListCache()
				storage.ClearObjectIDCachePending(projectRoot, cacheCtx.OldID)
				system.InvalidateValidationCacheForCacheContext(projectRoot, cacheCtx)
				return nil
			case pkgctx.CacheOperationInvalidateAndUpdate:
				system.InvalidateObjectIDCache(cacheCtx.OldID)
				storage.ClearObjectIDCachePending(projectRoot, cacheCtx.OldID)
				err = system.UpdateObjectIDCache(cacheCtx.NewID, cacheCtx.Kind, cacheCtx.FilePath)
				if err == nil {
					storage.InvalidateListCacheForKind(cacheCtx.Kind)
					storage.ClearObjectIDCachePending(projectRoot, cacheCtx.NewID)
				}
				system.InvalidateValidationCacheForCacheContext(projectRoot, cacheCtx)
				return err
			default:
				return nil
			}
		})

		// Set change notification handler ([REDACTED-ID])
		// Batch validation triggers and flush by size or time so SCH-val runs once per batch (see validation_trigger_batch.go).
		// No new scheduler_job is created—avoids recursion and one-off job proliferation (see BLI reusable validation job).
		storage.SetChangeNotificationHandler(func(ctx context.Context, operation, kind, id string, objectData map[string]any) error {
			if operation != storage.OpCreate && operation != storage.OpUpdate && operation != storage.OpDelete {
				return nil
			}
			projectRoot := cli.ResolveProjectRoot(".")
			if projectRoot == EmptyValue {
				return nil
			}
			AddValidationTrigger(projectRoot, kind, id, operation)
			// Run a narrow TRIGGER/FANOUT cascade so delete operations keep caches
			// (object-id-cache, list cache, validation cache) in sync with storage,
			// including background deletes/compactions that do not pass CacheContext.
			system.CascadeOnObjectChange(ctx, projectRoot, operation, kind, id, zqkenv.SessionID().Get())
			return nil
		})

		// Register dependency propagation subscriber so coordinator delivers lifecycle.dependency_ref events
		lifecycle.RegisterDependencyPropagationWithCoordinator(cli.GetObjectStorageForProjectRoot)
		// Lifecycle hook: append to lifecycle event WAL (for listener), dependency propagation, priority-plan complete updater, and scheduler jobs.
		storage.SetLifecycleHookHandler(func(ctx context.Context, kind, fromState, toState string, objectData map[string]any) error {
			projectRoot := pkgctx.GetLifecycleProjectRoot(ctx)

			// Append status transition so lifecycle listener can accumulate and trigger gated transitions
			if id, _ := objectData[objects.FieldKeyID].(string); id != EmptyValue {
				lifecycle.AppendStatusTransition(projectRoot, kind, id, fromState, toState)
			}
			// One-hop status listeners: outbound refs on the catalyst are stubs.
			if id, _ := objectData[objects.FieldKeyID].(string); id != EmptyValue && projectRoot != EmptyValue {
				// Keep propagation inside the transition's consistency boundary. Short-lived CLI
				// processes can exit before detached work applies the parent lifecycle edge.
				lifecycle.ApplyDependencyRefEvents(ctx, logging.NewEventLogger(ctx), provider, projectRoot, kind, id, fromState, toState, objectData)
			}
			// When a backlog_item completes: emit remaining-open drained criterion if the
			// container field is already 0. Synchronous so short-lived CLI does not miss WAL.
			// TRACK: BLI-CEF-CONTAINER-REMAINING-OPEN-001
			if kind == objects.KindBacklogItem && toState == "complete" && projectRoot != EmptyValue {
				if planRef, _ := objectData[objects.FieldKeyPriorityPlanRef].(string); planRef != EmptyValue {
					lifecycle.TryEmitRemainingOpenDrained(ctx, projectRoot, planRef, cli.GetObjectStorageForProjectRoot)
				}
				milRefs := make([]string, 0)
				milRef, _ := objectData[objects.FieldKeyMilestoneRef].(string)
				if milRef != "" {
					milRefs = append(milRefs, milRef)
				}
				if milRefsRaw, ok := objectData[objects.FieldKeyMilestoneRefs]; ok {
					for _, milID := range lifecycle.StringRefsFromAny(milRefsRaw) {
						if milID != "" && milID != milRef {
							milRefs = append(milRefs, milID)
						}
					}
				}
				for _, milID := range milRefs {
					lifecycle.TryEmitAllBacklogItemsCompleteForMilestone(ctx, projectRoot, milID, cli.GetObjectStorageForProjectRoot)
					lifecycle.TryEmitRemainingOpenDrained(ctx, projectRoot, milID, cli.GetObjectStorageForProjectRoot)
				}
			}
			// When a criterion reaches a satisfied status, milestones that reference it may become completable.
			if kind == objects.KindCriteria && projectRoot != EmptyValue && lifecycle.CriterionStatusMeetsMilestoneGateForMilestone(toState) {
				if critID, _ := objectData[objects.FieldKeyID].(string); critID != EmptyValue {
					lifecycle.TryEmitForMilestonesContainingCriterion(ctx, projectRoot, critID, cli.GetObjectStorageForProjectRoot)
					lifecycle.TryEmitForBacklogItemsContainingCriterion(ctx, projectRoot, critID, cli.GetObjectStorageForProjectRoot)
					if toState == "complete" {
						lifecycle.TryEmitForTestCasesContainingCriterion(ctx, projectRoot, critID, cli.GetObjectStorageForProjectRoot)
					}
				}
			}
			if kind == objects.KindPriorityPlan && toState == "complete" && projectRoot != EmptyValue {
				object.RunPriorityPlanCompleteUpdater(ctx, projectRoot)
			}
			// When priority_plan activates: trigger execution lock
			// TRACK: BLI-TDE-LIFECYCLE-PROMOTE-CLAIM-001
			if kind == objects.KindPriorityPlan && toState == objects.ObjectStatusActive && projectRoot != EmptyValue {
				if planID, _ := objectData[objects.FieldKeyID].(string); planID != EmptyValue {
					if provider != nil {
						_ = lifecycle.MaybeExecutionLockPlan(ctx, logging.NewEventLogger(ctx), provider, projectRoot, planID)
					}
				}
			}
			// Repoint SCH-cvs-pipeline-tick when the package-vetting convergence_session completes or a matching session activates.
			if kind == objects.KindConvergenceSession && projectRoot != EmptyValue {
				if st, ok := cli.GetObjectStorageForProjectRoot(projectRoot); ok && st != nil {
					_ = schedulerpkg.SyncCVSPipelineTickJobOnLifecycle(ctx, st, fromState, toState, objectData) //nolint:errcheck // best-effort
				}
			}
			// Immediate branch/worktree cleanup when a task or backlog item completes or reaches a terminal/error state.
			if (kind == objects.KindAgentTask || kind == objects.KindBacklogItem) && projectRoot != EmptyValue {
				isTerminal := false
				switch toState {
				case "complete", "completed", "implemented", "archived", "rejected", "error":
					isTerminal = true
				}
				if isTerminal {
					if id, _ := objectData[objects.FieldKeyID].(string); id != EmptyValue {
						goroutinelabels.NewGoroutine("immediate_worktree_cleanup", "clean up worktree and branch for completed/terminal item").
							StartSimple(func() {
								svc := maintenance.NewGitMaintenanceService(projectRoot)
								_ = svc.CleanupWorktreeAndBranchForID(ctx, id)
							})
					}
				}
			}
			if sched := schedulerpkg.GetGlobalScheduler(); sched != nil {
				_ = sched.TriggerJobByLifecycle(ctx, kind, fromState, toState, objectData) //nolint:errcheck // best-effort
			} else {
				// Offline queuing for lifecycle events when scheduler is down
				q := schedulerpkg.NewJobTriggerQueue(projectRoot)
				if q != nil {
					_ = q.EnqueueLifecycleTrigger(kind, fromState, toState, objectData)
				}
			}
			return nil
		})
	}

	// Notify coordinator when persisted current root is set (zqk use) so subscribers can react.
	clicontext.OnCurrentRootSet = func(workspaceRoot, newProjectRoot, previousProjectRoot string) {
		coord := coordination.GetCoordinator()
		if coord == nil {
			return
		}
		eventCtx := coordination.NewEventContext("current_root_set", coordination.OperationTypeProjectRootChanged, "complete").
			WithChannels(false, false, false, true). // Operational only; no audit/logging from use command
			WithEventData(&coordination.EventData{
				LoggingFields: []coordination.LoggingField{
					{Key: "workspace_root", Value: workspaceRoot},
					{Key: "new_project_root", Value: newProjectRoot},
					{Key: "previous_project_root", Value: previousProjectRoot},
				},
				AuditMetadata: map[string]any{
					"workspace_root":        workspaceRoot,
					"new_project_root":      newProjectRoot,
					"previous_project_root": previousProjectRoot,
				},
			})
		goroutinelabels.NewGoroutine("project_root_changed_emit", "emitting project_root_changed to coordinator").
			StartSimple(func() {
				_ = coord.Emit(pkgctx.NewSystemContext(), eventCtx) //nolint:errcheck // best-effort
			})
	}
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	// Set the real MCP Client Transport as the default for the CLI
	federation.SetDefaultTransport(meshbroker.NewMCPClientTransport(""))

	ensureCommandsRegistered()
	// Initialize timeout hook and metrics tracking
	hook := clitool.GetTimeoutHook()

	// Set up metrics store (skip for help/version commands to avoid blocking)
	// Check args to see if this is a help or version request
	args := os.Args[1:]
	isHelpOrVersion := false
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "help" || arg == "version" || arg == "--version" || arg == "-v" || arg == "completion" {
			isHelpOrVersion = true
			break
		}
	}

	if !isHelpOrVersion {
		projectRoot := cli.ResolveProjectRoot(".")
		if projectRoot != EmptyValue {
			metricsPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir, paths.CommandMetricsFile)
			metricsStore, err := clitool.NewFileMetricsStore(metricsPath)
			if err == nil {
				hook.SetMetricsStore(metricsStore)
			}
		}
	}

	// Wrap command execution with timeout and metrics
	ctx := pkgctx.NewSystemContext()
	// Use a brand-aware executable name for metrics and command tracking.
	// Default is derived from os.Args[0] (and can be overridden later by config in PersistentPreRunE).
	brand.SetExecutableName(filepath.Base(os.Args[0]))
	command := brand.ExecutableName()
	commandArgs := os.Args[1:]

	// Dispatch loop (single job): run with timeout + progress so CLI and MCP share the same path.
	// See docs/architecture/DISPATCH_LOOP_AND_MCP.md.
	err := hook.WrapCommand(ctx, command, commandArgs, func(execCtx context.Context) error {
		item := &dispatch.WorkItem{
			OperationType: "cli",
			ProjectRoot:   cli.ResolveProjectRoot("."),
			Profile:       profileHuman,
		}
		runner := func(ctx context.Context) error {
			rootCmd.SetArgs(commandArgs)
			rootCmd.SetContext(ctx)
			if ctx != nil {
				return rootCmd.ExecuteContext(ctx)
			}
			return rootCmd.Execute()
		}
		return dispatch.Run(execCtx, item, runner)
	})

	// Ensure profiling resources are stopped and written even if command aborted or timed out
	if cpuProfileFile != nil {
		pprof.StopCPUProfile()
		_ = cpuProfileFile.Close()
		cpuProfileFile = nil
	}
	if goroutineProfilePath != "" {
		_ = writeGoroutineProfile(goroutineProfilePath)
		goroutineProfilePath = ""
	}

	// Prevent uninterruptible-sleep (UE) cascade after command completion.
	//
	// Root cause: when stdout/stderr are connected to a full pipe or socket buffer
	// (e.g. IDE terminal socket, piped output), any goroutine that tries to write
	// to them enters uninterruptible sleep (UE) at the kernel level.  On macOS, a UE
	// process holds the binary's vm_object lock.  Every subsequent exec() of the same
	// binary then blocks at _dyld_start + 0, also entering UE state — cascading until
	// the machine must be rebooted.
	//
	// Two-layer defence applied after the main command output is fully written:
	//
	//   Layer 1 — Hard exit watchdog: if any cleanup operation (FlushAll, PersistentPostRunE
	//   audit events, error logging) blocks indefinitely, this goroutine forces the process
	//   to exit within 8 s.  It uses os.Exit so it kills goroutines in UE state too.
	//
	//   Layer 2 — O_NONBLOCK on stdout/stderr: converts future blocking writes from
	//   cleanup goroutines into Go-runtime async I/O (netpoller park, not kernel UE).
	//   Parked goroutines are killed cleanly when the process exits.
	exitCode := 0
	if err != nil {
		exitCode = 1
		var exitCoder interface{ ExitCode() int }
		if errors.As(err, &exitCoder) {
			exitCode = exitCoder.ExitCode()
		}
	}
	if !isHelpOrVersion {
		// Layer 1: watchdog goroutine.
		goroutinelabels.NewGoroutine("app", "exit watchdog").StartSimple(func() {
			time.Sleep(8 * time.Second)
			os.Exit(exitCode)
		})
		// Layer 2: mark stdout/stderr non-blocking.  The main command output has already
		// been written; remaining writes are cleanup/logging only and can tolerate EAGAIN.
		// syscall.SetNonblock is a thin wrapper around fcntl(O_NONBLOCK) — Unix only, but
		// this binary targets macOS/Linux.
		_ = syscall.SetNonblock(int(os.Stdout.Fd()), true) //nolint:errcheck,gosec // Best effort; G115: FD from os.Stdout
		_ = syscall.SetNonblock(int(os.Stderr.Fd()), true) //nolint:errcheck,gosec // Best effort; G115: FD from os.Stderr
	}

	// Ensure CAS index updates are flushed before process exit.
	//
	// CAS index writes are batched asynchronously for throughput. For CLI commands, it's important
	// that once the command returns, other processes can immediately read what was written/updated.
	// This also prevents tests from racing on index persistence.
	//
	// CRITICAL: Flush BEFORE error handling - otherwise os.Exit(1) prevents flush and creates
	// stale index entries (files written but index not persisted). See [REDACTED-ID].
	if !isHelpOrVersion {
		drainCtx, drainCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = storage.GetGlobalShutdownCoordinator().DrainAll(drainCtx)
		drainCancel()
		hook.Wait() // Wait for async metrics recording
	}

	if err != nil {
		// Determine logging profile from context flag or default to human
		// This ensures MCP mode errors go to stderr, not stdout
		profile := profileHuman // Default
		if contextFlag, flagErr := rootCmd.Flags().GetString("context"); flagErr == nil && contextFlag != EmptyValue {
			profile = contextFlag
		}

		// CRITICAL: In MCP mode (detected via environment variable), always use "mcp" profile
		// This ensures errors go to stderr, not stdout, preventing JSON-RPC pollution
		if zqkenv.MCPAccountID().Get() != EmptyValue {
			profile = profileMCP
		}

		// Use appropriate logging profile based on context
		logger := logging.GetLoggerFromProfile(profile)
		logging.Fluent(logger).Error("Command execution failed", err).Log()
		os.Exit(exitCode)
	}
}

func init() {
	// Initialize brand/executable name early so help text and examples constructed during init
	// can use the configured executable name.
	//
	// Priority:
	// 1) brand.executable_name in config/zqk-local.yaml then config/zqk.yaml
	// 2) actual executable name (os.Args[0])
	// 3) default ("zqk")
	execName := "zqk"
	if len(os.Args) > 0 && os.Args[0] != EmptyValue {
		execName = filepath.Base(os.Args[0])
	}
	projectRoot := cli.ResolveProjectRoot(".")
	namespacePrefix := strings.ToLower(execName)
	if projectRoot != EmptyValue {
		loaded := brand.LoadFromProject(projectRoot)
		if loaded.ExecutableName != EmptyValue {
			execName = loaded.ExecutableName
		}
		if loaded.ProductName != EmptyValue {
			brand.SetProductName(loaded.ProductName)
		}
		if loaded.NamespacePrefix != EmptyValue {
			namespacePrefix = loaded.NamespacePrefix
		}
	}

	brand.SetExecutableName(execName)
	namespacePrefix = brand.ProductNamespacePrefix(namespacePrefix)
	brand.SetNamespacePrefix(namespacePrefix)
	paths.CLICommandName = execName
	paths.SetNamespacePrefix(namespacePrefix)
	rootCmd.Use = execName

	// Set CLI version for compatibility checking
	// This allows the migration detector to verify CLI version compatibility
	detector.GetCLIVersion = func() string {
		return version
	}

	// Set version template (matches utility version command format)
	rootCmd.SetVersionTemplate(fmt.Sprintf("Executable: %s\nVersion: %s\nBuild datetime: %s\nCommit hash: %s\n", brand.ExecutableName(), version, buildDate, gitCommit))

	// Add context flag for context profiles (e.g., --context ai-agent)
	rootCmd.PersistentFlags().String("context", "", "Context profile (ai-agent, human, debug). Profiles set default output format and behavior.")

	// Add format flag as persistent so it's available on all commands
	// Format can also be inferred from --context profile (e.g., ai-agent -> json)
	rootCmd.PersistentFlags().StringP("format", "f", "", "Output format (json, yaml, table, json-rpc, stream). If not set, inferred from --context profile.")

	// Add timeout flag as persistent so it's available on all commands
	// Timeout can be overridden per-command for long-running operations
	rootCmd.PersistentFlags().Duration("timeout", 0, "Timeout for command execution (0 = auto-calculate based on command type and metrics)")

	// Hidden flag for integration tests to allow targeted teardowns
	rootCmd.PersistentFlags().String("test-id", "", "Test ID to embed in the process name (for integration tests only)")
	_ = rootCmd.PersistentFlags().MarkHidden("test-id")

	// CPU and goroutine profiles for performance/hang analysis (inherited globally by hot-path commands).
	// POL-CODE-015: keep developer instrumentation off Global Flags help.
	rootCmd.PersistentFlags().String("cpuprofile", "", "Write CPU profile to file")
	_ = rootCmd.PersistentFlags().MarkHidden("cpuprofile")
	rootCmd.PersistentFlags().String("cpu-profile", "", "Write CPU profile to file (alias for --cpuprofile)")
	_ = rootCmd.PersistentFlags().MarkHidden("cpu-profile")
	rootCmd.PersistentFlags().String("goroutine-profile", "", "Write goroutine profile to file (for hang/deadlock analysis)")
	_ = rootCmd.PersistentFlags().MarkHidden("goroutine-profile")
	rootCmd.PersistentFlags().String("goroutines-profile", "", "Write goroutine profile to file (alias for --goroutine-profile)")
	_ = rootCmd.PersistentFlags().MarkHidden("goroutines-profile")

	// Set version info for utility commands
	utility.SetVersionInfo(version, buildDate, gitCommit)

	// Initialize default format handlers
	// This registers table, json, yaml, json-rpc, and stream format handlers
	cli.InitializeDefaultHandlers()

	// Initialize builder registry on global spec loader for version-aware loading
	// This enables loading specs by version (e.g., ledger v1.0.0 vs v2.0.0)
	builders.InitializeGlobalSpecLoader()

	// Command registration (including dynamic kind subcommands) is deferred to ensureCommandsRegistered()
	// so tests can set ZQK_TEST_ROOT before NewRootCommand() and get correct kind routing.

	// Set up context loading for all commands
	rootCmd.PersistentPreRunE = rootPersistentPreRunE

	rootCmd.PersistentPostRunE = rootPersistentPostRunE
}

// rootPreRunInitFileLogging configures global file logging from profile/config (extracted to reduce gocyclo in rootPersistentPreRunE).
func rootPreRunInitFileLogging(cmd *cobra.Command, projectRoot string, isHelpCommand bool) {
	isInitCommand := false
	if len(os.Args) >= 3 && os.Args[1] == "system" && os.Args[2] == "init" {
		isInitCommand = true
	} else if len(os.Args) >= 2 && os.Args[1] == "init" {
		isInitCommand = true
	}
	if projectRoot == EmptyValue || isHelpCommand || isInitCommand {
		return
	}
	// Determine formatter based on profile/context
	// This determines both the formatter and the log file extension (.json vs .log)
	var formatter logging.Formatter

	// Check context flag first (highest precedence)
	profile := profileHuman // Default
	fileLogLevel := logging.InfoLevel
	if contextFlag := cli.GetFlagFromChain(cmd, "context"); contextFlag != nil && contextFlag.Value.String() != EmptyValue {
		profile = contextFlag.Value.String()
	} else {
		// Check if this is the MCP serve command (before checking config)
		// MCP serve should always use "mcp" profile for JSON logging
		if cmd.Parent() != nil && cmd.Parent().Name() == profileMCP && cmd.Name() == "serve" {
			profile = profileMCP
		} else {
			// Check config file for profile and logging.level
			// Try .zqk/config/config.yaml first, then .zqk/config.yaml (same order as GetErrorLogOutput)
			for _, rel := range []string{
				filepath.Join(paths.ProjectDataDir, paths.ConfigDir, paths.ProjectConfigFile),
				filepath.Join(paths.ProjectDataDir, paths.ProjectConfigFile),
			} {
				configPath := filepath.Join(projectRoot, rel)
				data, err := fileutil.ReadFile(configPath)
				if err != nil {
					continue
				}
				var config map[string]any
				if err := yaml.Unmarshal(data, &config); err != nil {
					continue
				}
				if profileVal, ok := config["profile"].(string); ok && profileVal != EmptyValue {
					profile = profileVal
				}
				if loggingSection, ok := config["logging"].(map[string]any); ok {
					if profileVal, ok := loggingSection["profile"].(string); ok && profileVal != EmptyValue {
						profile = profileVal
					}
					if levelStr, ok := loggingSection["level"].(string); ok && levelStr != EmptyValue {
						if lv, ok := logging.ParseLevel(levelStr); ok {
							fileLogLevel = lv
						}
					}
				}
				break // use first config file found
			}
		}
	}

	// Check for MCP mode (environment variable) - this is for subprocesses
	if zqkenv.MCPAccountID().Get() != EmptyValue {
		profile = profileMCP
	}

	// Globally enforce --verbose flag by upgrading human profile to debug profile
	if verboseFlag := cli.GetFlagFromChain(cmd, "verbose"); verboseFlag != nil && verboseFlag.Value.String() == "true" {
		if profile == profileHuman {
			profile = profileDebug
		}
	}

	// Determine formatter based on profile
	// JSON formatter: mcp, system, ai-agent -> log-events.json
	// Text formatter: human, debug -> log-events.log
	// Use command context for all formatters
	formatterCtx := cmd.Context()
	when.When(func() bool { return profile == profileMCP || profile == profileSystem || profile == profileAIAgent }).Then(func() {
		formatter = logging.NewJSONFormatter(formatterCtx)
	}).OrElseWhen(func() bool { return profile == profileDebug }).Then(func() {
		formatter = logging.NewCompactFormatter(formatterCtx)
	}).OrElse(func() {
		formatter = logging.NewTextFormatter(formatterCtx)
	}).Run()

	// Initialize router with formatter, profile, and file log level (from config logging.level or default info)
	if err := logging.InitializeGlobalRouter(projectRoot, formatter, profile, fileLogLevel); err != nil {
		// Log error but don't fail command - file logging is best-effort
		// Use a simple logger since router isn't initialized yet
		simpleLogger := logging.NewLogger(os.Stderr, logging.DebugLevel, logging.NewJSONFormatter(formatterCtx))
		logging.Fluent(simpleLogger).Warn("Failed to initialize file logging").
			WithError(err).
			Log()
	}
}

// rootPersistentPreRunE is extracted from init to keep cyclomatic complexity in bounds (gocyclo).
func rootPersistentPreRunE(cmd *cobra.Command, args []string) error {
	// Platform-independent orphan prevention: exit when MCP server (parent) dies
	parentDeathWatcherOnce.Do(startParentDeathWatcherIfSet)

	initStart := time.Now()
	mcpInitTrace(initStart, "pre_run_start")

	// Start CPU profiling if --cpuprofile or --cpu-profile was set (once per run)
	if cpuProfileFile == nil {
		var profName string
		if f := cli.GetFlagFromChain(cmd, "cpuprofile"); f != nil && f.Value.String() != EmptyValue {
			profName = f.Value.String()
		} else if f := cli.GetFlagFromChain(cmd, "cpu-profile"); f != nil && f.Value.String() != EmptyValue {
			profName = f.Value.String()
		}

		if profName != "" {
			if f, err := fileutil.Create(profName); err == nil {
				if err := pprof.StartCPUProfile(f); err == nil {
					cpuProfileFile = f
				} else {
					_ = f.Close()
				}
			}
		}
	}

	// Capture goroutine profile target if --goroutine-profile or --goroutines-profile was set
	if goroutineProfilePath == "" {
		if f := cli.GetFlagFromChain(cmd, "goroutine-profile"); f != nil && f.Value.String() != EmptyValue {
			goroutineProfilePath = f.Value.String()
		} else if f := cli.GetFlagFromChain(cmd, "goroutines-profile"); f != nil && f.Value.String() != EmptyValue {
			goroutineProfilePath = f.Value.String()
		}
	}
	// Optional: channel for background storage init (commands that require storage, e.g. object/internal)
	var objectStorageReady chan struct {
		p   storage.ObjectStorageProvider
		err error
	}
	// Detect help/version commands to avoid expensive initialization
	// Check multiple sources: cmd name, args passed to PreRunE, and os.Args
	isCompletion := false
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "completion" {
			isCompletion = true
			break
		}
	}
	isHelpCommand := cmd.Name() == "help" || cmd.Name() == "version" || cmd.Name() == "completion" || isCompletion ||
		argsContainHelpOrVersion(os.Args[1:]) || argsContainHelpOrVersion(args)

	isInitCommand := false
	if len(os.Args) >= 3 && os.Args[1] == "system" && os.Args[2] == "init" {
		isInitCommand = true
	} else if len(os.Args) >= 2 && os.Args[1] == "init" {
		isInitCommand = true
	}

	resolver := cli.ResolveProjectRoot
	if isInitCommand {
		resolver = func(startPath string) string { return "" }
	}

	// Initialize CLI context - bundles all initialization state
	// ResolveProjectRoot ensures explicit roots (ZQK_PROJECT_ROOT, ZQK_TEST_ROOT) override CWD.
	initCtx := pkgctx.NewCliInitializationContext(resolver, ".")
	projectRoot := initCtx.GetProjectRoot()
	mcpInitTrace(initStart, "project_root_resolved")

	// Allow CPU profile dump on SIGUSR2 (kill -USR2 <pid>); writes to projectRoot/.zqk/logs/cpu-<timestamp>.prof (or temp dir if no project)
	logsDir := fileutil.TempDir()
	if !isInitCommand && projectRoot != EmptyValue {
		logsDir = filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir)
	}
	diagnostics.StartCPUProfileOnSIGUSR2(logsDir)

	// Resolve orchestration requirements from the command that will run (annotations + defaults).
	// See internal/cli/command_requirements.go and docs/architecture/COMMAND_ORCHESTRATION.md.
	req := cli.GetCommandRequirements(cmd, args)

	// Pre-warm storage in background so expensive init overlaps with context load, scheduler
	// check, tracker setup, etc. Use the global storage provider cache so we create once per
	// project root per process; creating a new FileObjectStorage every time (no cache) caused
	// init to block in HashRegistry.Save and hung commands (e.g. bulk delete) before they ran.
	// Only when the target command requires storage (e.g. object CRUD/list); otherwise we avoid
	// starting storage and the CAS index queue (avoids blocking/hangs in e.g. generate-builders).
	if !isHelpCommand && !isInitCommand && req.RequiresStorage && projectRoot != EmptyValue {
		objectStorageReady = make(chan struct {
			p   storage.ObjectStorageProvider
			err error
		}, 1)
		root := projectRoot
		goroutinelabels.NewGoroutine("app", "storage initialization").StartSimple(func() {
			ctx := pkgctx.NewSystemContext()
			p, err := storage.GetGlobalStorageProviderCache().GetOrCreate(ctx, root)
			objectStorageReady <- struct {
				p   storage.ObjectStorageProvider
				err error
			}{p, err}
		})
	}

	// Initialize global log router with file destination if project root is available
	rootPreRunInitFileLogging(cmd, projectRoot, isHelpCommand)

	// For help/version commands, create a minimal no-op context with system defaults only
	// This keeps the pattern consistent without blocking on project/storage initialization
	if isHelpCommand {
		ctx, err := cli.GetContextFromCommand(cmd, initCtx)
		if err != nil {
			// If context creation fails, create minimal context with defaults
			manager := cli.NewContextManager()
			minimalCtx, err := manager.LoadContext(initCtx)
			if err == nil && minimalCtx != nil {
				cli.SetContext(cmd, cli.ContextFromInner(minimalCtx))
			}
		} else {
			cli.SetContext(cmd, ctx)
		}
		return nil
	}

	// Enforce Centralized Auth Middleware
	if !isInitCommand {
		if err := AuthMiddleware(cmd, projectRoot); err != nil {
			return err
		}
	}

	// Load context and apply to command (initCtx always provides valid project root)
	ctx, err := cli.GetContextFromCommand(cmd, initCtx)
	if err != nil {
		// Don't fail on context errors - create minimal context with defaults
		manager := cli.NewContextManager()
		minimalCtx, err := manager.LoadContext(initCtx)
		if err == nil && minimalCtx != nil {
			cli.SetContext(cmd, cli.ContextFromInner(minimalCtx))
		}
		return nil
	}

	// Store context in command for later use
	cli.SetContext(cmd, ctx)
	mcpInitTrace(initStart, "context_loaded")

	// Apply brand configuration from context so all subsequent rendering uses config, not hardcoded strings.
	// If not provided, fall back to the runtime-executable name.
	if ctx != nil && ctx.Context != nil {
		if ctx.ExecutableName != EmptyValue {
			brand.SetExecutableName(ctx.ExecutableName)
			paths.CLICommandName = ctx.ExecutableName
		}
		if ctx.ProductName != EmptyValue {
			brand.SetProductName(ctx.ProductName)
		}
		if ctx.NamespacePrefix != EmptyValue {
			brand.SetNamespacePrefix(ctx.NamespacePrefix)
			paths.SetNamespacePrefix(ctx.NamespacePrefix)
		}
	}
	cli.ApplyBrandingToCommandTree(rootCmd, brand.ExecutableName(), brand.ProductName())

	// Project root is already available from initCtx above

	// Check scheduler daemon status when the target command requires it (see command_requirements).
	// Commands that manage the scheduler (start/stop/status) set RequiresSchedulerCheck=false.
	if !isHelpCommand && req.RequiresSchedulerCheck {
		if err := checkSchedulerDaemonStatus(projectRoot, cmd, ctx); err != nil {
			return err
		}
	}

	// Check for CLI reminder (interrupt notification) - but skip for help/version
	// This ensures reminders are visible even during long-running operations
	// Run synchronously but quickly (just file I/O) so it appears before command output
	if !isHelpCommand && projectRoot != EmptyValue {
		checkCLIReminderInRoot(projectRoot)
		if err := checkPolicyInterruptGateInRoot(projectRoot); err != nil {
			return err
		}
	}

	// Initialize command execution tracker for comprehensive tracking
	tracker := clitool.NewCommandExecutionTracker()

	// Extract command information
	// Use CommandPath() directly - it already includes the full command path (e.g., "zqk system check")
	// Don't prepend "zqk" as CommandPath() already includes it
	fullCommand := cmd.CommandPath()
	tracker.SetCommand(fullCommand, args)
	tracker.SetNormalizedCommand(clitool.NormalizeCommand(fullCommand, args))

	// Extract flags
	flags := clitool.ExtractFlagsFromCommand(cmd)
	tracker.SetFlags(flags)

	// Extract context information
	tracker.SetContext(ctx.PriorityPlan, ctx.Workstream, ctx.Milestone)

	// Set actor information (system context for CLI)
	secCtx := pkgctx.NewSystemSecurityContext()
	tracker.SetActor(secCtx.AccountID, secCtx.Roles)

	// Store tracker in command context for access during execution
	cmdCtx := cmd.Context()
	if cmdCtx == nil {
		cmdCtx = pkgctx.NewSystemContext()
	}
	cmd.SetContext(clitool.WithTracker(cmdCtx, tracker))

	// Use pre-warmed storage when the target command required it (objectStorageReady set above).
	if objectStorageReady != nil && !isHelpCommand {
		mcpInitTrace(initStart, "storage_wait_start")
		result := <-objectStorageReady
		mcpInitTrace(initStart, "storage_ready")
		if result.err == nil {
			if cli.OnStorageCreated != nil {
				cli.OnStorageCreated(result.p)
			}
			// Register in cli process cache so lifecycle hooks that use GetObjectStorageForProjectRoot
			// (e.g. TryEmitRemainingOpenDrained) see storage; context alone is insufficient.
			cli.RegisterStorageForProjectRoot(projectRoot, result.p)
			cmd.SetContext(cli.WithStorageProvider(cmd.Context(), result.p))
		}
	}

	// STRICT MANDATE: ALL commands (except help) MUST establish or reuse a valid ZQK session,
	// unless explicitly opted out via RequireSession(false).
	// This enforces the architectural integrity of the system and eliminates brittle snowflake logic.
	// (Test bypass: Test suites running against ZQK_TEST_ROOT mock their own lifecycles)
	isTestRoot := zqkenv.TestRoot().Get() != EmptyValue
	if !isHelpCommand && !isInitCommand && projectRoot != EmptyValue && !isTestRoot && req.RequiresSession {
		if p := cli.GetStorageProvider(cmd.Context()); p != nil {
			if sp, ok := p.(storage.ObjectStorageProvider); ok {
				title := cmd.CommandPath()
				accountID := secCtx.AccountID
				sessionID, reused := TryReuseSession(cmd.Context(), projectRoot, title, accountID, sp)
				if !reused {
					sessionID = StartZqkSession(cmd.Context(), projectRoot, title, accountID, sp)
					if sessionID != EmptyValue {
						WritePersistedSessionID(cmd.Context(), projectRoot, sessionID, accountID, sp)
					}
				}
				if sessionID != EmptyValue {
					cmd.SetContext(WithZqkSessionID(cmd.Context(), sessionID))
				}
			}
		}
	}

	mcpInitTrace(initStart, "pre_run_done")
	return nil
}

func rootPersistentPostRunE(cmd *cobra.Command, args []string) error {
	// Stop CPU profiling and close file if we started it
	if cpuProfileFile != nil {
		pprof.StopCPUProfile()
		_ = cpuProfileFile.Close()
		cpuProfileFile = nil
	}
	// Write goroutine profile if requested
	if goroutineProfilePath != "" {
		_ = writeGoroutineProfile(goroutineProfilePath)
		goroutineProfilePath = ""
	}
	// Skip tracking for help/version commands
	isHelpCommand := cmd.Name() == "help" || cmd.Name() == "version"
	if isHelpCommand {
		return nil
	}

	// Get tracker from context
	cmdCtx := cmd.Context()
	if cmdCtx == nil {
		return nil
	}

	tracker := clitool.GetTrackerFromContext(cmdCtx)
	if tracker == nil {
		return nil
	}

	// Touch session (updated_at, title) only when storage is already available; throttled to avoid WAL storm when many invocations share one session.
	if sessionID := GetZqkSessionIDFromContext(cmdCtx); sessionID != EmptyValue && cli.StorageAvailableForOptionalUse(cmd) {
		projectRoot := cli.ResolveProjectRoot(".")
		if projectRoot != EmptyValue {
			if p := cli.GetStorageProvider(cmd.Context()); p != nil {
				if sp, ok := p.(storage.ObjectStorageProvider); ok {
					accountID := tracker.ActorID
					if accountID == EmptyValue {
						accountID = pkgctx.SystemAccountID
					}
					TouchSessionIfNotThrottled(cmdCtx, projectRoot, sessionID, cmd.CommandPath(), accountID, sp)
				}
			}
		}
	}

	// Get execution result (success/failure)
	// Note: We can't get the actual error here, but we can infer success from the fact
	// that PostRunE was called (if there was an error, RunE would have returned it)
	// For now, we'll set success=true and let the actual error be captured elsewhere
	tracker.SetOutcome(true, 0, nil, false)

	// Convert tracker to metric and record
	metric := tracker.ToCommandMetric()

	// Record metrics if metrics store is available
	hook := clitool.GetTimeoutHook()
	if hook != nil {
		// Get metrics store from hook (it's set in Execute())
		// We need to access it, but it's private. For now, we'll record via the hook's method
		// Actually, we should use the hook's RecordCommandExecution method if available
		// But since we're in PostRunE, the command has already executed
		// The timeout hook's WrapCommand already recorded basic metrics
		// We need to enhance those metrics with the additional context

		// For now, we'll create an audit event with the full tracking information
		// The metrics store update can happen asynchronously
	}

	// Create audit event only when storage is already available (ignorable operation:
	// use existing component, never create — see docs/architecture/COMMAND_ORCHESTRATION.md vital vs ignorable).
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot != EmptyValue && cli.StorageAvailableForOptionalUse(cmd) {
		profile := profileHuman
		if c := cli.GetContext(cmd); c != nil && c.Profile != EmptyValue {
			profile = c.Profile
		}
		createCommandAuditEvent(cmd, projectRoot, metric, profile)
	}

	return nil
}

// argsContainHelpOrVersion returns true if any of the given args is a help or version flag.
func argsContainHelpOrVersion(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "help", "--help", "-h", "version", "--version", "-v":
			return true
		}
	}
	return false
}

// checkCLIReminderInRoot checks for CLI reminder and displays it prominently
// This runs in PersistentPreRunE so it appears before every command
func checkCLIReminderInRoot(projectRoot string) {
	if projectRoot == EmptyValue {
		return
	}

	flagFile := filepath.Join(projectRoot, paths.ProjectDataDir, "cli_reminder.flag")
	if info, err := fileutil.Stat(flagFile); err == nil {
		// Only show if file is recent (within last hour) to avoid spam
		if time.Since(info.ModTime()) < time.Hour {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(logger).Warn("CLI-FIRST REMINDER (INTERRUPT NOTIFICATION)").Log()

			if content, err := fileutil.ReadFile(flagFile); err == nil {
				logging.Fluent(logger).Info(string(content)).Log()
			}

			modTime := info.ModTime()
			age := time.Since(modTime)
			logging.Fluent(logger).Info("Reminder last updated").
				String("last_updated", zqktime.FormatLayoutUTC(modTime, zqktime.LayoutDateTimeSpace)).
				String("age", formatReminderDuration(age)).
				Log()
		}
	}
}

// checkPolicyInterruptGateInRoot short-circuits CLI flows for non-system actors when a critical policy
// interrupt is pending and unacknowledged. Background/system flows should not be blocked.
// Actor identity is best-effort: prefer explicit env vars; default is system.
func checkPolicyInterruptGateInRoot(projectRoot string) error {
	if projectRoot == EmptyValue {
		return nil
	}
	actorID := zqkenv.AccountID().Get()
	if actorID == EmptyValue {
		actorID = zqkenv.MCPAccountID().Get()
	}
	if actorID == EmptyValue || actorID == pkgctx.SystemAccountID {
		return nil
	}

	acks, err := policyinterrupt.LoadAcksIncremental(projectRoot)
	if err != nil {
		return nil // best-effort gate; do not block on WAL read errors
	}
	latest, err := policyinterrupt.LoadLatestCriticalUnacked(projectRoot, acks)
	if err != nil || latest == nil {
		return nil
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	msg := latest.Message
	if msg == EmptyValue {
		msg = "Critical policy interrupt pending"
	}
	logging.Fluent(logger).Warn("CRITICAL POLICY INTERRUPT (ACK REQUIRED)").
		String("message", msg).
		String("dedupe_key", latest.DedupeKey).
		String("policy_id", latest.PolicyID).
		String("action", "Run: zqk system policy-interrupts ack --dedupe-key "+latest.DedupeKey).
		Log()
	return errfmt.Errorf("critical policy interrupt requires acknowledgement: %s", latest.DedupeKey)
}

func formatReminderDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

// checkSchedulerDaemonStatus checks if scheduler daemon is running and warns users if not
// Only shows warning to users with appropriate permissions
func checkSchedulerDaemonStatus(projectRoot string, cmd *cobra.Command, _ *cli.Context) error {
	// Skip if no project root
	if projectRoot == EmptyValue {
		return nil
	}
	// Skip when running under test root (e.g. bootstrap CRUD test subprocess) to avoid
	// slow or flaky process/pid checks and to keep test isolation
	if zqkenv.TestRoot().Get() != EmptyValue {
		return nil
	}

	// Check if user has permission to read scheduler status
	// Use system context for permission check
	secCtx := pkgctx.NewSystemSecurityContext()

	// Check for read:scheduler_job or manage:scheduler permission
	hasPermission := false
	for _, role := range secCtx.Roles {
		if role == "admin" {
			hasPermission = true
			break
		}
	}

	if !hasPermission {
		for _, perm := range secCtx.Permissions {
			if perm == "read:scheduler_job" || perm == "manage:scheduler" || perm == "read:*" || perm == "manage:*" {
				hasPermission = true
				break
			}
		}
	}

	if !hasPermission {
		return nil // Don't show warning if user doesn't have permission
	}

	// Check if scheduler is running (in this process or another).
	// Use GetGlobalSchedulerIfAvailable so we never block on globalSchedulerMu (daemon holds it
	// during job dispatch); blocking here left CLI processes stuck and unkillable in some environments.
	sched, ok := schedulerpkg.GetGlobalSchedulerIfAvailable()
	var isRunning bool
	when.When(func() bool { return ok && sched != nil && sched.IsRunning() }).Then(func() {
		isRunning = true
	}).OrElse(func() {
		running, _, err := schedulerpkg.IsSchedulerRunning(projectRoot)
		isRunning = err == nil && running
	}).Run()

	if !isRunning && isMutatingCommand(cmd) {
		ignore, _ := cmd.Flags().GetBool("allow-degraded")

		if !ignore {
			// POL-CODE-007: user-facing output via logger
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(logger).Warn("Scheduler daemon is down. Operations will be queued to WAL.").
				String("impact", "Side-effects and lifecycles will execute when daemon returns").
				String("action", "Start with: zqk scheduler start").
				Log()
		}
	}
	return nil
}

// isMutatingCommand returns true when cmd (or any ancestor) is a mutating command
// that depends on the scheduler for side-effects and lifecycle coordination.
// Read-only commands (get, list, count, status, etc.) return false so the
// root-level scheduler guard does not emit noisy false-positive warnings.
func isMutatingCommand(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "create", "update", "delete", "workflow", "system":
			return true
		}
	}
	return false
}

// createCommandAuditEvent creates an audit event for command execution
// Coordinator emits asynchronously, so this function always succeeds
func createCommandAuditEvent(cmd *cobra.Command, projectRoot string, metric *clitool.CommandMetric, profile string) {
	if cmd == nil || projectRoot == EmptyValue {
		return
	}
	operation := buildOperationDescription(metric)
	severity := determineSeverity(metric)
	metadata := buildAuditMetadata(projectRoot, metric)
	//nolint:errcheck // Audit events are fire-and-forget
	_ = validateAndWriteAuditEvent(cmd, projectRoot, operation, severity, metadata, metric, profile)
}

// registerCommands registers all implemented commands
// Commands are organized into logical groups with common and specialized subcommands
func registerCommands() {
	rootCmd.AddGroup(&cobra.Group{ID: "everyday", Title: "Everyday Commands:"})
	rootCmd.AddGroup(&cobra.Group{ID: "integrations", Title: "Integrations & Automation:"})
	rootCmd.AddGroup(&cobra.Group{ID: "advanced", Title: "Advanced Knowledge Kernel:"})
	rootCmd.AddGroup(&cobra.Group{ID: "admin", Title: "Administration:"})

	// TRACK: TDE-1789637124469147000-98c2f37f — do not LoadFields before every command.
	// Object operations group (CRUD, query, and management)
	objCmd := object.NewObjectCmd()
	object.RegisterDynamicKindCommands(objCmd)
	objCmd.GroupID = "everyday"
	rootCmd.AddCommand(objCmd)

	// System operations group (health, validation, and maintenance)
	systemCmdInst := commsystem.NewSystemCmd()
	systemCmdInst.GroupID = "everyday"
	rootCmd.AddCommand(systemCmdInst)

	// In-process code search engine (grep / zgrep)
	grepCmdInst := grep.NewGrepCmd()
	grepCmdInst.GroupID = "everyday"
	rootCmd.AddCommand(grepCmdInst)

	// Documentation management group
	docmanCmdInst := docman.NewDocmanCmd()
	docmanCmdInst.GroupID = "advanced"
	rootCmd.AddCommand(docmanCmdInst)

	// Graph operations group (Enterprise Only)

	// Ambient operations group (Enterprise Only)
	// Internal operations group (Enterprise Only)

	// Utility operations group (version, migration, and helpers)
	utilityCmdInst := utility.NewUtilityCmd()
	utilityCmdInst.GroupID = "admin"
	rootCmd.AddCommand(utilityCmdInst)
	// Top-level `version` — spec `.zqk/cli/specs/root/version_command.yaml` (same Run as utility version).
	versionCmdInst := utility.NewRootVersionCmd()
	versionCmdInst.GroupID = "everyday" // Give it a more appropriate group
	rootCmd.AddCommand(versionCmdInst)

	// Automation and integration group (Enterprise Only)
	// Pre-commit background results (Enterprise Only)

	// Quickstart / start-here zero-friction onboarding guide
	quickstartCmdInst := system.NewQuickstartCmd()
	quickstartCmdInst.GroupID = "everyday"
	rootCmd.AddCommand(quickstartCmdInst)

	// Tray: named shortcuts to zqk argv (.zqk/tray.yaml over embedded defaults)
	trayCmdInst := tray.NewTrayCmd()
	trayCmdInst.GroupID = "everyday"
	rootCmd.AddCommand(trayCmdInst)

	// Draft templates for object create / scenario bundles (spec-driven builders)
	newcmdCmdInst := newcmd.NewNewCmd()
	newcmdCmdInst.GroupID = "everyday"
	rootCmd.AddCommand(newcmdCmdInst)

	// Reports group (Enterprise Only)
	// Traceability matrices (Enterprise Only)

	// MCP server group
	mcpCmdInst := mcp.NewMCPCmd()
	mcpCmdInst.GroupID = "integrations"
	rootCmd.AddCommand(mcpCmdInst)

	// Scheduler daemon management (start, stop, status, trigger, service, etc.)
	schedulerCmdInst := commscheduler.NewSchedulerCmd()
	schedulerCmdInst.GroupID = "integrations"
	rootCmd.AddCommand(schedulerCmdInst)

	// Community test dashboard (Studio cmd/zqk/test is not on this SKU).
	testCmdInst := commtest.NewTestCmd()
	testCmdInst.GroupID = "everyday"
	rootCmd.AddCommand(testCmdInst)

	// Observer, Keystore, Callback, Rollback, Semantic, Spec, Org, Domain, Ontology (Enterprise Only)

	// Workflow guidance commands
	workflowCmdInst := workflow.NewWorkflowCmd()
	workflowCmdInst.GroupID = "everyday"
	rootCmd.AddCommand(workflowCmdInst)

	// Auth, Join, Mesh (Enterprise Only)

	// Shell completion (bash, zsh, fish)
	rootCmd.AddCommand(NewCompletionCmd(rootCmd))

	// Multi-agent orchestration (Enterprise Only)

	// Domain-specific commands (future)
	// rootCmd.AddCommand(domain.NewBacklogCmd())
	// rootCmd.AddCommand(domain.NewGoalCmd())
	// rootCmd.AddCommand(domain.NewMilestoneCmd())
	// rootCmd.AddCommand(domain.NewWorkstreamCmd())
	// rootCmd.AddCommand(domain.NewPriorityPlanCmd())

	// Filter out admin commands if we are not running as the zqk-admin binary (or a test binary)
	baseArg := filepath.Base(os.Args[0])
	if !strings.HasPrefix(baseArg, "zqk-admin") && !strings.HasSuffix(baseArg, ".test") {
		var toRemove []*cobra.Command
		for _, cmd := range rootCmd.Commands() {
			if cmd.GroupID == "admin" {
				toRemove = append(toRemove, cmd)
			}
		}
		rootCmd.RemoveCommand(toRemove...)
	}
}
