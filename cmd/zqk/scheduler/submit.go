package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"github.com/spf13/cobra"
)

const (
	submitJobIDPrefix          = "SCH-%d"
	submitExecutionModeOneTime = "one_time"
	submitStatusActive         = "active"
	submitOriginProject        = "zqk"
	submitOriginSystem         = "zqk"
)

const submitSystemAccountID = pkgctx.SystemAccountID

// NewSubmitCmd creates the submit command
func NewSubmitCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Submit a command as a background job to the scheduler",
		"Submit a command to run as a background job in the scheduler.",
		"",
		"This creates a one-time scheduler job that executes immediately. The job runs",
		"in the background and you can check its status using '%s scheduler activity'",
		"or '%s scheduler history'.",
		"",
		"The command is executed using run_wrapper, which provides:",
		"- Progress tracking (stdout/stderr capture)",
		"- Timeout management",
		"- Retry logic",
		"- Execution metrics",
	).
		AddExample("Submit a simple command", "%s scheduler submit \"echo hello world\"").
		AddExample("Submit a command with arguments", "%s scheduler submit \"zqk system aggregate-audit\" --window 24h --delete").
		AddExample("Submit with timeout and working directory", "%s scheduler submit \"make build\" --max-runtime 600 --workdir /path/to/project").
		AddExample("Submit with environment variables", "%s scheduler submit \"npm test\" --env \"NODE_ENV=test\" --env \"CI=true\"").
		AddExample("Submit with retry on failure", "%s scheduler submit \"curl https://api.example.com/data\" --retry 3").
		ExcludeCommonFlags()

	submitCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSchedulerSubmitCommandBuilder(), &cobra.Command{
		Use:  "submit <command> [args...]",
		Args: cobra.MinimumNArgs(1),
	})
	cli.BindAsyncProgress(submitCmd, runSubmit)

	// Apply help builder to command
	helpBuilder.ApplyToCommand(submitCmd)

	// Flags may already come from DNA (scheduler/submit_command.yaml). Only add
	// when the builder is still a stub so generate-command-builders does not panic
	// on "flag redefined". TRACK: BLI-1785903708509306000-a6d8dc5b
	ensureSubmitFlags(submitCmd)

	cli.AddCommonFlags(submitCmd)
	return submitCmd
}

func ensureSubmitFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	if f.Lookup("max-runtime") == nil {
		f.IntP("max-runtime", "t", 0, "Maximum runtime in seconds (0 = no timeout, default: 3600)")
	}
	if f.Lookup("workdir") == nil {
		f.StringP("workdir", "w", "", "Working directory for command execution")
	}
	if f.Lookup("env") == nil {
		// Must match generated builder (StringArray). GetStringSlice silently
		// returns empty when the flag was registered as StringArray — that
		// dropped every --env on the live CLI path.
		// TRACK: BLI-1786687873940250000-a6c3a985 — swarm LLM env inheritance via submit.
		f.StringArrayP("env", "e", []string{}, "Environment variables (format: KEY=VALUE, can be specified multiple times)")
	}
	if f.Lookup("retry") == nil {
		f.IntP("retry", "r", 0, "Number of retry attempts on failure (default: 0)")
	}
	if f.Lookup("retry-delay") == nil {
		f.Int("retry-delay", 5, "Delay between retries in seconds (default: 5)")
	}
	if f.Lookup("title") == nil {
		f.String("title", "", "Job title (default: auto-generated from command)")
	}
	if f.Lookup("description") == nil {
		f.String("description", "", "Job description (default: auto-generated)")
	}
}

func runSubmit(cmd *cobra.Command, args []string) error {
	submitCtx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 30*time.Second)
	defer cancel()

	cliCtx := cli.GetContext(cmd)
	if cliCtx == nil {
		return errfmt.Errorf("failed to get context")
	}

	done := make(chan error, 1)
	bud := goroutinelabels.DefaultBudget()
	submitBuilder := goroutinelabels.NewGoroutine("scheduler_submit_job", "submitting job to scheduler")
	if bud != nil {
		submitBuilder = submitBuilder.WithBudget(bud)
	}
	submitBuilder.StartSimple(func() {
		done <- submitJob(cliCtx, cmd, args)
	})

	select {
	case err := <-done:
		return err
	case <-submitCtx.Done():
		diagnosticsDir := zqkenv.DiagnosticsDir().Get()
		if diagnosticsDir == emptyValue {
			projectRoot := cli.ResolveProjectRoot(".")
			if projectRoot == emptyValue && cliCtx != nil {
				projectRoot = cliCtx.ProjectRoot
			}
			if projectRoot != emptyValue {
				diagnosticsDir = fmt.Sprintf("%s/.zqk/diagnostics", projectRoot)
			}
		}
		if diagnosticsDir != emptyValue {
			if err := captureDiagnostics(diagnosticsDir, "scheduler_submit_timeout"); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: Failed to capture diagnostics: %v\n", err)
			}
		}
		return errfmt.Errorf("scheduler submit timed out after 30 seconds - operation may be hanging. Diagnostics captured to %s", diagnosticsDir)
	}
}

func submitJob(cliCtx *cli.Context, cmd *cobra.Command, args []string) error {
	ctx := pkgctx.NewSystemContext()

	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue && cliCtx != nil {
		projectRoot = cliCtx.ProjectRoot
	}
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	// Create storage provider
	storageFactory, err := storage.NewStorageFactory(ctx, projectRoot)
	if err != nil {
		return errfmt.Newf("failed to create storage factory").Wrap(err)
	}
	storageProvider := storageFactory.GetStorage()

	secCtx := pkgctx.NewSystemSecurityContext()

	// Parse command and arguments
	command := args[0]
	commandArgs := args[1:]

	// Parse flags
	timeout, _ := cmd.Flags().GetInt("max-runtime")
	if timeout == 0 {
		timeout = 3600 // Default 1 hour timeout
	}
	workdir, _ := cmd.Flags().GetString("workdir")
	envSlice := submitEnvFlagValues(cmd)
	retryCount, _ := cmd.Flags().GetInt("retry")
	retryDelay, _ := cmd.Flags().GetInt("retry-delay")
	title, _ := cmd.Flags().GetString("title")
	description, _ := cmd.Flags().GetString("description")
	callbackCompletion, _ := cmd.Flags().GetString("callback-completion")
	callbackFailure, _ := cmd.Flags().GetString("callback-failure")

	// Parse environment variables
	envVars := make(map[string]string)
	for _, envStr := range envSlice {
		parts := strings.SplitN(envStr, "=", 2)
		if len(parts) != 2 {
			return errfmt.Errorf("invalid environment variable format: %s (expected KEY=VALUE)", envStr)
		}
		envVars[parts[0]] = parts[1]
	}

	// Use nanosecond resolution so concurrent CLI processes cannot overwrite a
	// submission merely because they started within the same wall-clock second.
	now := time.Now().UTC()
	jobID := newSubmitJobID(now)

	// Generate title and description if not provided
	if title == emptyValue {
		title = fmt.Sprintf("Background job: %s", command)
		if len(commandArgs) > 0 {
			argCount := 3
			if len(commandArgs) < 3 {
				argCount = len(commandArgs)
			}
			title += " " + strings.Join(commandArgs[:argCount], " ")
			if len(commandArgs) > 3 {
				title += "..."
			}
		}
	}
	if description == emptyValue {
		cmdLine := command
		if len(commandArgs) > 0 {
			cmdLine += " " + strings.Join(commandArgs, " ")
		}
		description = fmt.Sprintf("Background job submitted via CLI: %s", cmdLine)
	}

	// Build job data
	jobData := map[string]any{
		objects.FieldKeyID:                jobID,
		objects.FieldKeyKind:              schedulerKindJob,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:            submitStatusActive,
		objects.FieldKeyJobType:           schedulerJobTypeRunWrapper,
		objects.FieldKeyTriggerType:       schedulerTriggerImmediate,
		objects.FieldKeyCategory:          schedulerpkg.CategoryManual,
		objects.FieldKeyPriority:          schedulerpkg.JobPriorityHigh,
		objects.FieldKeyExecutionMode:     submitExecutionModeOneTime,
		objects.FieldKeyMaxRuntimeSeconds: timeout,
		objects.FieldKeyEnabled:           true,
		objects.FieldKeyTitle:             title,
		objects.FieldKeyDescription:       description,
		objects.FieldKeyCommand:           command,
		objects.FieldKeyCommandArgs:       commandArgs,
		objects.FieldKeyRetryCount:        retryCount,
		objects.FieldKeyRetryDelaySeconds: retryDelay,
		objects.FieldKeyCreatedAt:         now.Format(time.RFC3339),
		objects.FieldKeyCreatedBy:         submitSystemAccountID,
		objects.FieldKeyUpdatedAt:         now.Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:         submitSystemAccountID,
		objects.FieldKeyOriginProject:     submitOriginProject,
		objects.FieldKeyOriginSystem:      submitOriginSystem,
	}

	if workdir != emptyValue {
		jobData[objects.FieldKeyWorkingDirectory] = workdir
	}
	if len(envVars) > 0 {
		jobData[objects.FieldKeyEnvironmentVariables] = envVars
	}
	if callbackCompletion != emptyValue {
		jobData[objects.FieldKeyCallbackOnCompletion] = callbackCompletion
	}
	if callbackFailure != emptyValue {
		jobData[objects.FieldKeyCallbackOnError] = callbackFailure
	}

	// Check if a job with this ID already exists (for one_time jobs, reuse existing)
	// This prevents creating duplicate objects - instead update the existing job
	existingJobs, listErr := storageProvider.List(ctx, secCtx, pkgctx.NewStorageContext(), storage.ListFilter{
		Kind:    schedulerKindJob,
		Filters: map[string]any{objects.FieldKeyID: jobID},
		Limit:   1,
	})

	var existingJob map[string]any
	if listErr == nil && len(existingJobs.Objects) > 0 {
		existingJob = existingJobs.Objects[0]
		executionMode, _ := existingJob[objects.FieldKeyExecutionMode].(string)

		// For one_time jobs, update the existing job instead of creating a new one
		if executionMode == submitExecutionModeOneTime {
			// Update the existing job: reset enabled, revive archived→active, update timestamps/command
			updates := map[string]any{
				objects.FieldKeyEnabled:   true,
				objects.FieldKeyStatus:    submitStatusActive,
				objects.FieldKeyPriority:  schedulerpkg.JobPriorityHigh,
				objects.FieldKeyUpdatedAt: now.Format(time.RFC3339),
				objects.FieldKeyUpdatedBy: submitSystemAccountID,
			}

			// Update command/args if provided (allows re-running with different commands)
			if command != emptyValue {
				updates[objects.FieldKeyCommand] = command
			}
			if len(commandArgs) > 0 {
				updates[objects.FieldKeyCommandArgs] = commandArgs
			}
			if title != emptyValue {
				updates[objects.FieldKeyTitle] = title
			}
			if description != emptyValue {
				updates[objects.FieldKeyDescription] = description
			}
			if timeout > 0 {
				updates[objects.FieldKeyMaxRuntimeSeconds] = timeout
			}
			if workdir != emptyValue {
				updates[objects.FieldKeyWorkingDirectory] = workdir
			}
			if len(envVars) > 0 {
				updates[objects.FieldKeyEnvironmentVariables] = envVars
			}
			if callbackCompletion != emptyValue {
				updates[objects.FieldKeyCallbackOnCompletion] = callbackCompletion
			}
			if callbackFailure != emptyValue {
				updates[objects.FieldKeyCallbackOnError] = callbackFailure
			}

			// Use the id field value as the object ID for Update()
			// For CAS objects, storage.Update() will handle the ID->hash mapping internally
			if err := storageProvider.Update(ctx, secCtx, jobID, updates); err != nil {
				return errfmt.Newf("failed to update existing scheduler job").Wrap(err)
			}
			flushSubmittedSchedulerJob(storageProvider, projectRoot)

			out := clipkg.NewParagraphBuilder().
				AddLinef("Reusing existing job: %s (updated for new run)", jobID).
				AddLineWithIndentf(2, "Command: %s", command).
				AddIf(len(commandArgs) > 0, " "+strings.Join(commandArgs, " "))
			out.AddLine("").AddLineWithIndentf(2, "Timeout: %d seconds", timeout).
				BlankLine().
				AddLinef("View status: zqk scheduler activity --job-id %s", jobID).
				AddLinef("View history: zqk scheduler history --job-id %s", jobID)
			if err := cli.WriteOutput(cmd, []byte(out.Build())); err != nil {
				return err
			}

			// Trigger the job
			triggerQueue := schedulerpkg.NewJobTriggerQueue(projectRoot)
			if err := triggerQueue.EnqueueTriggerRequestWithOrigin(jobID, schedulerpkg.TriggerOriginCLISubmit); err != nil {
				return errfmt.Newf("failed to enqueue trigger request").Wrap(err)
			}

			return nil
		}
		// For reusable jobs, if one exists with same ID, that's an error (should be unique)
		return errfmt.Errorf("scheduler job with ID %s already exists (execution_mode: %s). Use a different ID or update the existing job", jobID, executionMode)
	}

	// No existing job found - create new one
	// Create the job (retry once if ID collision occurs)
	var createErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			jobID = newSubmitJobID(time.Now().UTC())
			jobData[objects.FieldKeyID] = jobID
		}
		createErr = storageProvider.Create(ctx, secCtx, jobData)
		if createErr == nil {
			break
		}
		// If object exists, retry with new ID
		if errors.Is(createErr, storage.ErrObjectExists) || strings.Contains(createErr.Error(), "already exists") {
			continue
		}
		// Other error, return it
		return errfmt.Newf("failed to create scheduler job").Wrap(createErr)
	}
	if createErr != nil {
		return errfmt.Newf("failed to create scheduler job after retries").Wrap(createErr)
	}
	flushSubmittedSchedulerJob(storageProvider, projectRoot)

	// Immediate jobs only run once the daemon receives a trigger request; without
	// this the job object is created but never executes (no history, no metrics).
	triggerQueue := schedulerpkg.NewJobTriggerQueue(projectRoot)
	if err := triggerQueue.EnqueueTriggerRequestWithOrigin(jobID, schedulerpkg.TriggerOriginCLISubmit); err != nil {
		return errfmt.Newf("failed to enqueue trigger request").Wrap(err)
	}

	// Output job ID (POL-CODE-007 / MCP: cli.WriteOutput for test capture)
	out := clipkg.NewParagraphBuilder().
		AddLinef("Job submitted: %s", jobID).
		AddLineWithIndentf(2, "Command: %s", command).
		AddIf(len(commandArgs) > 0, " "+strings.Join(commandArgs, " ")).
		AddLine("").AddLineWithIndentf(2, "Timeout: %d seconds", timeout)
	if workdir != emptyValue {
		out.AddLineWithIndentf(2, "Working directory: %s", workdir)
	}
	if retryCount > 0 {
		out.AddLineWithIndentf(2, "Retries: %d (delay: %d seconds)", retryCount, retryDelay)
	}
	out.BlankLine().
		AddLinef("View status: zqk scheduler activity --job-id %s", jobID).
		AddLinef("View history: zqk scheduler history --job-id %s", jobID)

	return cli.WriteOutput(cmd, []byte(out.Build()))
}

func newSubmitJobID(now time.Time) string {
	return fmt.Sprintf(submitJobIDPrefix, now.UnixNano())
}

func flushSubmittedSchedulerJob(provider storage.ObjectStorageProvider, projectRoot string) {
	flushCtx, cancel := storage.DurabilityFlushContext()
	defer cancel()
	_ = storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, provider, projectRoot, []string{schedulerKindJob})
}

// submitEnvFlagValues reads --env from the live command flag type.
// Generated CLI builders register StringArray; some unit tests still register
// StringSlice. Prefer GetStringArray, then fall back so both paths work.
func submitEnvFlagValues(cmd *cobra.Command) []string {
	if values, err := cmd.Flags().GetStringArray("env"); err == nil {
		return values
	}
	values, _ := cmd.Flags().GetStringSlice("env")
	return values
}
