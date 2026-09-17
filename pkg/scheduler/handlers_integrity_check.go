package scheduler

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lanceman/zqk/pkg/execwrap"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"github.com/lanceman/zqk/internal/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/when"
)

// Log events for integrity_check handler (POL-CODE-007 stable keys).
const (
	LogEventIntegrityCheckJobStart           = JobTypeIntegrityCheck + "_job_start"
	LogEventIntegrityCheckProcessing         = JobTypeIntegrityCheck + "_processing"
	LogEventIntegrityCheckCommandFailed      = JobTypeIntegrityCheck + "_command_failed"
	LogEventIntegrityCheckCriticalViolations = JobTypeIntegrityCheck + "_critical_violations"
	LogEventIntegrityCheckJobCompleted       = JobTypeIntegrityCheck + "_job_completed"
)

// integrityCheckCommandArgs is the scheduler subprocess for cache/hash repair.
// --fast (or --check-refs=false) cannot combine with --auto-fix/--force
// (check_fast_contract). Fixes still batch via --auto-fix-scheduler.
func integrityCheckCommandArgs() []string {
	return []string{"system", "check", "--auto-fix", "--auto-fix-scheduler"}
}

// IntegrityCheckHandler handles integrity check jobs for caches and hash indexes
type IntegrityCheckHandler struct {
	storage storagepkg.ObjectStorageProvider
	logger  logging.Logger
}

// NewIntegrityCheckHandler creates a new integrity check handler
// NewIntegrityCheckHandler creates a new integrity check handler
func NewIntegrityCheckHandler(storage storagepkg.ObjectStorageProvider) IntegrityCheckHandlerInterface {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	return &IntegrityCheckHandler{
		storage: storage,
		logger:  logger,
	}
}

// Execute executes integrity checks on caches and hash indexes
func (h *IntegrityCheckHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunIntegrityCheckViaPipeline(ctx, h, job)
}

// executeIntegrityCheckCore executes integrity checks.
// Called from RunIntegrityCheckViaPipeline NORMALIZE stage.
func (h *IntegrityCheckHandler) executeIntegrityCheckCore(ctx context.Context, job *ScheduledJob) error {
	SLog(h.logger).Info(LogEventIntegrityCheckJobStart).
		JobID(job.ID).
		Log()

	// Get project root from storage instance (most reliable - uses explicit project root)
	// Priority: storage instance > auto-discovery (no environment variables)
	var projectRoot string
	if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
		projectRoot = fileStorage.GetProjectRoot()
	}

	if projectRoot == emptyValue {
		// Fallback: try to find project root from current working directory
		wd, err := fileutil.Getwd()
		if err != nil {
			return errfmt.Newf("failed to get working directory").Wrap(err)
		}
		projectRoot = cli.ResolveProjectRoot(wd)
		if projectRoot == emptyValue {
			return errfmt.Errorf("project root not found - storage must be initialized with explicit project root")
		}
	}

	// Get event data if provided (for targeted checks)
	eventData := ctx.Value(evtDataKey{})
	var targetKinds []string
	var targetIDs []string
	if eventData != nil {
		eventDataMap, ok := eventData.(map[string]any)
		if ok {
			if kindsInterface, ok := eventDataMap["kinds"]; ok {
				switch v := kindsInterface.(type) {
				case []string:
					targetKinds = v
				case []any:
					for _, k := range v {
						if kStr, ok := k.(string); ok {
							targetKinds = append(targetKinds, kStr)
						}
					}
				}
			}
			if idsInterface, ok := eventDataMap["object_ids"]; ok {
				switch v := idsInterface.(type) {
				case []string:
					targetIDs = v
				case []any:
					for _, id := range v {
						if idStr, ok := id.(string); ok {
							targetIDs = append(targetIDs, idStr)
						}
					}
				}
			}
		}
	}

	// Build check command arguments
	// This will check:
	// 1. Object ID cache integrity (stale entries, missing files)
	// 2. Hash registry integrity (missing hashes, hash mismatches)
	// 3. Duplicate ID detection
	// 4. Cache consistency
	// 5. Reference integrity
	// 6. Lifecycle compliance
	// Use --auto-fix-scheduler to batch fixes via scheduler (default: true, but make explicit)
	// This allows validation to complete quickly while fixes run in background
	checkArgs := integrityCheckCommandArgs()

	// Pass timeout flag to override timeout hook's 5-minute cap
	// Use job's max_runtime_seconds, but leave some buffer for command overhead
	// Default to 25 minutes if not set (leaves 5 min buffer from 30 min job timeout)
	timeoutSeconds := job.MaxRuntimeSeconds
	when.When(func() bool { return timeoutSeconds <= 0 }).Then(func() {
		timeoutSeconds = 1500 // 25 minutes default
	}).OrElse(func() {
		// Leave 5% buffer for command overhead and cleanup
		timeoutSeconds = int(float64(timeoutSeconds) * 0.95)
		if timeoutSeconds < 60 {
			timeoutSeconds = 60 // Minimum 1 minute
		}
	}).Run()
	checkArgs = append(checkArgs, "--timeout", fmt.Sprintf("%ds", timeoutSeconds))

	if len(targetIDs) > 0 {
		// Check specific objects
		checkArgs = append(checkArgs, targetIDs...)
	} else if len(targetKinds) > 0 {
		// Check specific kinds
		checkArgs = append(checkArgs, targetKinds...)
	}
	// If neither targetIDs nor targetKinds, check all (default behavior)

	// Find zqk binary deterministically for scheduler subprocesses.
	zqkBin := resolveSchedulerCLIBinary(projectRoot)

	// Execute the check command
	SLog(h.logger).Info(LogEventIntegrityCheckProcessing).
		JobID(job.ID).
		ProjectRoot(projectRoot).
		String("command", fmt.Sprintf("%s %s", zqkBin, strings.Join(checkArgs, " "))).
		String("target_kinds", strings.Join(targetKinds, ",")).
		String("target_ids", strings.Join(targetIDs, ",")).
		Log()

	cmd := execwrap.CommandContext(ctx, zqkBin, checkArgs...)
	cmd.Dir = projectRoot
	// Set project root in environment for the command
	// Note: This is for the subprocess, not for our own context switching
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("%s=%s", zqkenv.ProjectRoot().Name(), projectRoot),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check command failed - log output and return error
		SLog(h.logger).Error(LogEventIntegrityCheckCommandFailed, err).
			JobID(job.ID).
			ProjectRoot(projectRoot).
			String("output", string(output)).
			Log()
		// Single-line error only: newlines in err.Error() break the text log formatter
		// (error=... splits across lines and merges with subsequent key=value fields).
		// Command output is already in the structured log above.
		return errfmt.Newf("integrity check failed").Wrap(err)
	}

	// Check for violations in output (even if command succeeded)
	outputStr := string(output)
	if strings.Contains(outputStr, "Tier 1") || strings.Contains(outputStr, "blocking") {
		// Critical violations found
		SLog(h.logger).Warn(LogEventIntegrityCheckCriticalViolations).
			JobID(job.ID).
			ProjectRoot(projectRoot).
			String("output", outputStr).
			Log()
		// Don't return error - violations were detected and logged
		// Auto-fix may have resolved some issues
	}

	SLog(h.logger).Info(LogEventIntegrityCheckJobCompleted).
		JobID(job.ID).
		ProjectRoot(projectRoot).
		String("output_length", fmt.Sprintf("%d", len(outputStr))).
		Log()

	return nil
}
