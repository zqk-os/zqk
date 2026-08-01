package system

import (
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	scsfacade "github.com/lanceman/zqk/ext/facade/scs"
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/objects"
)

const (
	backlogStatusComplete  = "complete"
	statusInProgress       = "in_progress"
	statusActive           = "active"
	sortByActiveOrder      = "active_order"
	priorityPlanIDPrefix   = "pri-"
	featureBranchPrefix    = "feature/pri-"
	gitCheckoutCreateFmt   = "git checkout -b %s"
	gitCheckoutSwitchFmt   = "git checkout %s"
	syncFlagPull           = "pull"
	syncFlagPush           = "push"
	syncFlagVerify         = "verify"
	syncFlagStage          = "stage"
	syncFlagCommit         = "commit"
	syncFlagAutoCommit     = "auto-commit"
	syncFlagMessage        = "message"
	syncFlagSkipCheck      = "skip-check"
	syncFlagCreatePR       = "create-pr"
	syncFlagPRTitle        = "pr-title"
	syncFlagPRBody         = "pr-body"
	syncFlagHelpPull       = "Pull changes from remote"
	syncFlagHelpPush       = "Push changes to remote"
	syncFlagHelpVerify     = "Verify signatures"
	syncFlagHelpStage      = "Stage all changes"
	syncFlagHelpCommit     = "Commit staged changes (requires --message or --auto-commit)"
	syncFlagHelpAutoCommit = "Auto-generate commit message and commit"
	syncFlagHelpMessage    = "Commit message (required if --commit without --auto-commit)"
	syncFlagHelpSkipCheck  = "Skip system health check (not recommended)"
	syncFlagHelpCreatePR   = "Create Pull Request (GitHub) or Merge Request (GitLab) after push"
	syncFlagHelpPRTitle    = "PR/MR title (auto-generated from priority plan if not provided)"
	syncFlagHelpPRBody     = "PR/MR body (auto-generated from priority plan if not provided)"
)

// NewSyncCmd creates a new sync command
func NewSyncCmd() *cobra.Command {
	var (
		pull       bool
		push       bool
		verify     bool
		stage      bool
		commit     bool
		autoCommit bool
		message    string
		skipCheck  bool
		createPR   bool
		prTitle    string
		prBody     string
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Sync objects between backends or environments",
		"Sync objects between backends or environments with branch management.",
		"",
		"This command synchronizes local state with remote repository using Git.",
		"It can pull changes from remote, push local changes, and verify signatures.",
		"",
		"Enhanced features:",
		"  - Automatic branch creation based on current priority plan",
		"  - Staging and committing changes in accordance with branch policies",
		"  - Automatic commit message generation based on changes",
		"  - System health check enforcement before commit/push (POLICY-CODE-004, POLICY-CODE-005)",
		"  - Pull Request/Merge Request creation (GitHub/GitLab) per priority plan (POLICY-WORKFLOW-002)",
		"",
		"Branch Policy:",
		"  - One feature branch per priority plan (e.g., feature/pri-210-phase-4)",
		"  - Automatically creates branch if it doesn't exist",
		"  - Ensures you're on the correct branch for the current priority plan",
		"  - If no active priority plan exists, branch management is skipped (use --message for commits)",
		"",
		"System Health Check (POLICY-CODE-004, POLICY-CODE-005):",
		"  - Automatically runs before commit/push operations (enabled by default)",
		"  - Blocks commit/push if Tier 1 (blocking) violations are detected",
		"  - Warns if Tier 2 (warnings) threshold is exceeded (threshold: 0)",
		"  - Use --skip-check to bypass (not recommended)",
	).
		AddExample("Pull changes from remote", "%s system sync --pull").
		AddExample("Stage, commit, and push changes", "%s system sync --stage --commit --push").
		AddExample("Auto-commit with generated message", "%s system sync --auto-commit --push").
		AddExample("Custom commit message", "%s system sync --commit --message \"Implement ITEM-657 and ITEM-659\"").
		AddExample("Dry run to see what would happen", "%s system sync --stage --auto-commit --push --dry-run").
		AddExample("Create PR/MR after push", "%s system sync --stage --auto-commit --push --create-pr").
		AddExample("Create PR with custom title and body", "%s system sync --push --create-pr --pr-title \"Custom Title\" --pr-body \"Custom body\"").
		ExcludeCommonFlags()

	syncCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemSyncCommandBuilder(), &cobra.Command{
		Use: "sync",
	})
	cli.BindAsyncProgress(syncCmd, func(cmd *cobra.Command, args []string) error {
		checkHealth := !skipCheck
		return runSync(cmd, pull, push, verify, stage, commit, autoCommit, message, checkHealth, createPR, prTitle, prBody)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(syncCmd)

	syncCmd.Flags().BoolVar(&pull, syncFlagPull, false, syncFlagHelpPull)
	syncCmd.Flags().BoolVar(&push, syncFlagPush, false, syncFlagHelpPush)
	syncCmd.Flags().BoolVar(&verify, syncFlagVerify, false, syncFlagHelpVerify)
	syncCmd.Flags().BoolVar(&stage, syncFlagStage, false, syncFlagHelpStage)
	syncCmd.Flags().BoolVar(&commit, syncFlagCommit, false, syncFlagHelpCommit)
	syncCmd.Flags().BoolVar(&autoCommit, syncFlagAutoCommit, false, syncFlagHelpAutoCommit)
	syncCmd.Flags().StringVar(&message, syncFlagMessage, "", syncFlagHelpMessage)

	// System health check flags (POLICY-CODE-004, POLICY-CODE-005)
	syncCmd.Flags().BoolVar(&skipCheck, syncFlagSkipCheck, false, syncFlagHelpSkipCheck)

	// PR/MR creation flags (POLICY-WORKFLOW-002)
	syncCmd.Flags().BoolVar(&createPR, syncFlagCreatePR, false, syncFlagHelpCreatePR)
	syncCmd.Flags().StringVar(&prTitle, syncFlagPRTitle, "", syncFlagHelpPRTitle)
	syncCmd.Flags().StringVar(&prBody, syncFlagPRBody, "", syncFlagHelpPRBody)

	cli.AddValidationFlags(syncCmd) // Adds --dry-run and --force flags

	return syncCmd
}

func runSync(cmd *cobra.Command, pull, push, verify, stage, commit, autoCommit bool, message string, checkHealth bool, createPR bool, prTitle, prBody string) error {
	// Use Processor pattern for context processing
	processor, err := cli.NewProcessor(cmd)
	if err != nil {
		return errfmt.Newf("failed to initialize processor").Wrap(err)
	}

	logger := processor.Logger()
	dryRun := cli.IsDryRun(cmd)
	projectRoot := processor.ProjectRoot()
	storageProvider := processor.Storage()

	if dryRun {
		logging.FluentEvent(logger).Info("DRY RUN MODE: No changes will be made").Log()
	}

	// Validate git repository
	if err := validateGitRepository(projectRoot, logger); err != nil {
		return err
	}

	// Handle priority plan and branch management
	currentPlan, err := handlePriorityPlanAndBranch(processor, projectRoot, autoCommit, message, logger, dryRun)
	if err != nil {
		return err
	}

	// Handle health check
	if err := handleHealthCheck(checkHealth, commit, autoCommit, push, projectRoot, logger, dryRun); err != nil {
		return err
	}

	// Handle staging
	if err := handleStaging(stage, cmd, projectRoot, logger, dryRun); err != nil {
		return err
	}

	// Handle committing
	if err := handleCommitting(commit, autoCommit, message, cmd, projectRoot, storageProvider, processor, currentPlan, logger, dryRun); err != nil {
		return err
	}

	// Handle pulling
	if err := handlePulling(pull, projectRoot, verify, logger, dryRun); err != nil {
		return err
	}

	// Handle pushing and PR creation
	if err := handlePushing(push, createPR, cmd, projectRoot, verify, storageProvider, processor, currentPlan, prTitle, prBody, logger, dryRun); err != nil {
		return err
	}

	return nil
}

// findCurrentPriorityPlan finds the current priority plan (reuses logic from pplan.go)
func findCurrentPriorityPlan(cmdCtx context.Context, storageProvider storage.ObjectStorageProvider) (map[string]any, error) {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	logger := logging.GetLoggerFromContext(cmdCtx)

	// First, try to find in_progress plan
	filter := storage.ListFilter{
		Kind:    objects.KindPriorityPlan,
		Filters: map[string]any{objects.FieldKeyStatus: statusInProgress},
	}

	result, err := storageProvider.List(cmdCtx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf("failed to query priority plans").Wrap(err)
	}

	if len(result.Objects) > 0 {
		if logger != nil {
			planID, _ := result.Objects[0][objects.FieldKeyID].(string)
			logging.FluentEvent(logger).Debug(fmt.Sprintf("Found in_progress plan: %s", planID)).Log()
		}
		return result.Objects[0], nil
	}

	// If no in_progress, find active plans
	filter = storage.ListFilter{
		Kind:    objects.KindPriorityPlan,
		Filters: map[string]any{objects.FieldKeyStatus: statusActive},
		SortBy:  sortByActiveOrder,
		SortAsc: true,
	}

	result, err = storageProvider.List(cmdCtx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf("failed to query active priority plans").Wrap(err)
	}

	if len(result.Objects) == 0 {
		return nil, errfmt.Errorf("no active or in_progress priority plans found")
	}

	if logger != nil {
		planID, _ := result.Objects[0][objects.FieldKeyID].(string)
		logging.FluentEvent(logger).Debug(fmt.Sprintf("Found active plan: %s", planID)).Log()
	}
	return result.Objects[0], nil
}

// generateBranchName generates a branch name from priority plan ID
// Converts PLAN-210 -> feature/pri-210-phase-4 (lowercase, with phase suffix if present)
func generateBranchName(planID string) string {
	// Convert to lowercase
	branchName := strings.ToLower(planID)

	// Replace PLAN- with feature/pri-
	branchName = strings.Replace(branchName, priorityPlanIDPrefix, featureBranchPrefix, 1)

	// If it's just PLAN-XXX, try to extract phase info from title if available
	// For now, we'll use the plan ID as-is and let the user customize if needed

	return branchName
}

// ensureBranch ensures we're on the correct branch, creating it if needed
func ensureBranch(projectRoot, expectedBranch string, logger *logging.EventLogger, dryRun bool) error {
	gf := scsfacade.New(projectRoot)
	// Get current branch
	currentBranch, err := gf.CurrentBranch()
	if err != nil {
		return errfmt.Newf("failed to determine current branch").Wrap(err)
	}

	// If already on the correct branch, nothing to do
	if currentBranch == expectedBranch {
		logging.FluentEvent(logger).Info("Already on correct branch").
			String("branch", expectedBranch).
			Log()
		return nil
	}

	// Check if branch exists
	if !gf.BranchExists(expectedBranch) {
		// Branch doesn't exist, create it
		if dryRun {
			logging.FluentEvent(logger).Info(cli.DryRunWould("create new branch")).
				String("branch", expectedBranch).
				Log()
			logging.FluentEvent(logger).Info(cli.DryRunWouldRunf(gitCheckoutCreateFmt, expectedBranch)).Log()
		} else {
			logging.FluentEvent(logger).Info("Creating new branch").
				String("branch", expectedBranch).
				Log()
			if output, err := gf.CreateBranch(expectedBranch); err != nil {
				return errfmt.Errorf("failed to create branch %s: %s", expectedBranch, string(output))
			}
			logging.FluentEvent(logger).Info("Created and switched to branch").
				String("branch", expectedBranch).
				Log()
		}
	} else {
		// Branch exists, switch to it
		if dryRun {
			logging.FluentEvent(logger).Info(cli.DryRunWould("switch to branch")).
				String("branch", expectedBranch).
				Log()
			logging.FluentEvent(logger).Info(cli.DryRunWouldRunf(gitCheckoutSwitchFmt, expectedBranch)).Log()
		} else {
			logging.FluentEvent(logger).Info("Switching to branch").
				String("branch", expectedBranch).
				Log()
			if output, err := gf.CheckoutBranch(expectedBranch); err != nil {
				return errfmt.Errorf("failed to switch to branch %s: %s", expectedBranch, string(output))
			}
			logging.FluentEvent(logger).Info("Switched to branch").
				String("branch", expectedBranch).
				Log()
		}
	}

	return nil
}

// gitStage stages all changes
func gitStage(cmd *cobra.Command, projectRoot string, logger *logging.EventLogger, dryRun bool) error {
	gf := scsfacade.New(projectRoot)
	if dryRun {
		// Show what would be staged
		output, err := gf.StatusShort()
		if err == nil {
			unstaged := strings.TrimSpace(string(output))
			if unstaged != emptyValue {
				logging.FluentEvent(logger).Info(cli.DryRunWould("stage the following changes:")).Log()
				//nolint:errcheck // Output errors are non-critical
				_ = cli.WriteOutput(cmd, []byte(unstaged+"\n"))
			} else {
				logging.FluentEvent(logger).Info(cli.DryRunMessage("No changes to stage")).Log()
			}
		}
		logging.FluentEvent(logger).Info(cli.DryRunWouldRun("git add -A")).Log()
		return nil
	}

	logging.FluentEvent(logger).Info("Staging all changes...").Log()

	output, err := gf.AddAll()
	if err != nil {
		return errfmt.Errorf("git add failed: %s", string(output))
	}

	logging.FluentEvent(logger).Info("Changes staged successfully").Log()
	return nil
}

// generateCommitMessage generates a commit message based on changes
func generateCommitMessage(projectRoot string, _ storage.ObjectStorageProvider, _ context.Context, currentPlan map[string]any, _ *logging.EventLogger) (string, error) {
	// Get list of changed files
	changedFiles, err := getChangedFiles(projectRoot)
	if err != nil {
		return "", err
	}

	// Extract backlog item IDs from changed files
	backlogItems := extractBacklogItems(changedFiles)

	// Build commit message from parts
	fileCount := len(changedFiles)
	message := buildCommitMessageParts(currentPlan, backlogItems, fileCount)

	return message, nil
}

// gitCommit commits staged changes
func gitCommit(cmd *cobra.Command, projectRoot, message string, logger *logging.EventLogger, dryRun bool) error {
	gf := scsfacade.New(projectRoot)
	if dryRun {
		// Show what would be committed
		output, err := gf.DiffCachedStat()
		if err == nil {
			diff := strings.TrimSpace(string(output))
			if diff != emptyValue {
				logging.FluentEvent(logger).Info(cli.DryRunWould("commit the following changes:")).Log()
				//nolint:errcheck // Output errors are non-critical
				_ = cli.WriteOutput(cmd, []byte(diff+"\n"))
			}
		}
		logging.FluentEvent(logger).Info(cli.DryRunWould("commit with message:")).
			String("message", message).
			Log()
		logging.FluentEvent(logger).Info(cli.DryRunWouldRun("git commit -m \"" + message + "\"")).Log()
		return nil
	}

	logging.FluentEvent(logger).Info("Committing changes...").
		String("message", message).
		Log()

	output, err := gf.Commit(message)
	if err != nil {
		return errfmt.Errorf("git commit failed: %s", string(output))
	}

	logging.FluentEvent(logger).Info("Changes committed successfully").Log()
	return nil
}

func gitPull(projectRoot string, verify bool, logger *logging.EventLogger, dryRun bool) error {
	gf := scsfacade.New(projectRoot)
	// Get current branch name
	currentBranch, err := gf.CurrentBranch()
	if err != nil {
		return errfmt.Newf("failed to determine current branch").Wrap(err)
	}

	if dryRun {
		logging.FluentEvent(logger).Info(cli.DryRunWould("pull changes from remote")).
			String("branch", currentBranch).
			Log()
		logging.FluentEvent(logger).Info(cli.DryRunWouldRun("git pull")).Log()
		if verify {
			logging.FluentEvent(logger).Info(cli.DryRunWould("verify signatures")).Log()
		}
		return nil
	}

	logging.FluentEvent(logger).Info("Pulling changes from remote...").
		String("branch", currentBranch).
		Log()

	// git pull uses the current branch's tracking configuration
	// If branch has upstream set: pulls from that remote/branch
	// If no upstream: uses default remote (usually 'origin') and current branch name
	output, err := gf.Pull(verify)
	if err != nil {
		return errfmt.Errorf("git pull failed: %s", string(output))
	}

	logging.FluentEvent(logger).Info("Pull completed successfully").
		String("branch", currentBranch).
		Log()
	return nil
}

func gitPush(cmd *cobra.Command, projectRoot string, verify bool, logger *logging.EventLogger, dryRun bool) error {
	gf := scsfacade.New(projectRoot)
	// Get current branch name
	currentBranch, err := gf.CurrentBranch()
	if err != nil {
		return errfmt.Newf("failed to determine current branch").Wrap(err)
	}

	if dryRun {
		// Show what would be pushed
		logOutput, err := gf.CommitsAheadOneline(currentBranch)
		if err == nil {
			commits := strings.TrimSpace(string(logOutput))
			if commits != emptyValue {
				logging.FluentEvent(logger).Info(cli.DryRunWould("push the following commits:")).Log()
				//nolint:errcheck // Output errors are non-critical
				_ = cli.WriteOutput(cmd, []byte(commits+"\n"))
			} else {
				logging.FluentEvent(logger).Info(cli.DryRunMessage("No commits to push")).Log()
			}
		}
		logging.FluentEvent(logger).Info(cli.DryRunWould("push changes to remote")).
			String("branch", currentBranch).
			Log()
		logging.FluentEvent(logger).Info(cli.DryRunWouldRun("git push")).Log()
		if verify {
			logging.FluentEvent(logger).Info(cli.DryRunWould("verify signatures")).Log()
		}
		return nil
	}

	logging.FluentEvent(logger).Info("Pushing changes to remote...").
		String("branch", currentBranch).
		Log()

	// git push uses the current branch's tracking configuration
	// If branch has upstream set: pushes to that remote/branch
	// If no upstream: uses default remote (usually 'origin') and current branch name
	output, err := gf.Push(verify)
	if err != nil {
		return errfmt.Errorf("git push failed: %s", string(output))
	}

	logging.FluentEvent(logger).Info("Push completed successfully").
		String("branch", currentBranch).
		Log()
	return nil
}

// runSystemHealthCheck runs system check and enforces POLICY-CODE-004 and POLICY-CODE-005
// Returns error if Tier 1 (blocking) violations are found
func runSystemHealthCheck(projectRoot string, logger *logging.EventLogger, dryRun bool) error {
	if dryRun {
		logging.FluentEvent(logger).Info(cli.DryRunWould("run system health check (POLICY-CODE-004, POLICY-CODE-005)")).Log()
		return nil
	}

	logging.FluentEvent(logger).Info("Running system health check (POLICY-CODE-004, POLICY-CODE-005)...").Log()

	// Find the zqk binary (should be in project root)
	zqkBin := filepath.Join(projectRoot, paths.CLICommandName)
	if _, err := os.Stat(zqkBin); err != nil {
		// Try to find it in PATH
		zqkBin = paths.CLICommandName
	}

	// Run system check with JSON output for parsing
	cmd := exec.Command(zqkBin, "system", "check", "--format", "json")
	zqkenv.WireExecForIsolatedProject(cmd, projectRoot)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// If command failed, try to parse output anyway (might have violations)
		logging.FluentEvent(logger).Warn("System check command returned error").
			WithError(err).
			Log()
	}

	// Parse JSON output to extract violation counts
	var checkResult struct {
		Summary struct {
			BlockingIssues   int `json:"blocking_issues"`
			Warnings         int `json:"warnings"`
			PublicBlocking   int `json:"public_blocking"`
			InternalBlocking int `json:"internal_blocking"`
		} `json:"summary"`
	}

	// Try to parse JSON from output
	outputStr := string(output)
	// Find JSON in output (might have log messages before/after)
	jsonStart := strings.Index(outputStr, "{")
	if jsonStart >= 0 {
		jsonEnd := strings.LastIndex(outputStr, "}")
		if jsonEnd > jsonStart {
			jsonStr := outputStr[jsonStart : jsonEnd+1]
			if err := json.Unmarshal([]byte(jsonStr), &checkResult); err != nil {
				// If JSON parsing fails, try to extract from table output
				logging.FluentEvent(logger).Warn("Failed to parse JSON output, checking table format").
					WithError(err).
					Log()
				return parseTableOutput(outputStr, logger)
			}
		} else {
			return parseTableOutput(outputStr, logger)
		}
	} else {
		return parseTableOutput(outputStr, logger)
	}

	// Check Tier 1 violations (zero tolerance per POLICY-CODE-005)
	if checkResult.Summary.BlockingIssues > 0 {
		err := errfmt.Errorf("tier 1 (blocking) violations detected: %d total (%d public, %d internal). resolve violations before committing (POLICY-CODE-004, POLICY-CODE-005)",
			checkResult.Summary.BlockingIssues,
			checkResult.Summary.PublicBlocking,
			checkResult.Summary.InternalBlocking)
		logging.FluentEvent(logger).Error("Tier 1 (Blocking) violations detected", err).
			Total(checkResult.Summary.BlockingIssues).
			BlockingPublic(checkResult.Summary.PublicBlocking).
			BlockingInternal(checkResult.Summary.InternalBlocking).
			Log()
		logging.FluentEvent(logger).Info("POLICY-CODE-004: All system check violations must be resolved before committing").Log()
		logging.FluentEvent(logger).Info("POLICY-CODE-005: Zero tolerance for Tier 1 violations").Log()
		logging.FluentEvent(logger).Info("").Log()
		logging.FluentEvent(logger).Info("To view details:").Log()
		logging.FluentEvent(logger).Info(fmt.Sprintf("  %s system check --verbose", paths.CLICommandName)).Log()
		logging.FluentEvent(logger).Info("").Log()
		logging.FluentEvent(logger).Info("To resolve:").Log()
		logging.FluentEvent(logger).Info(fmt.Sprintf("  %s system check --auto-fix --force", paths.CLICommandName)).Log()
		return err
	}

	// Check Tier 2 warnings threshold (0 = zero tolerance)
	if checkResult.Summary.Warnings > 0 {
		logging.FluentEvent(logger).Warn("Tier 2 (Warnings) threshold exceeded").
			Int("count", checkResult.Summary.Warnings).
			Int("threshold", 0).
			Log()
		logging.FluentEvent(logger).Info("POLICY-CODE-005: Tier 2 warnings should be reviewed before merge").Log()
		logging.FluentEvent(logger).Info(fmt.Sprintf("To view details: %s system check --verbose", paths.CLICommandName)).Log()
		// Don't fail on Tier 2, just warn (per policy, only Tier 1 blocks)
	}

	if checkResult.Summary.BlockingIssues == 0 && checkResult.Summary.Warnings == 0 {
		logging.FluentEvent(logger).Info("System health check passed").
			Int("tier1", checkResult.Summary.BlockingIssues).
			Int("tier2", checkResult.Summary.Warnings).
			Log()
	}

	return nil
}

// parseTableOutput attempts to extract violation counts from table-format output
func parseTableOutput(output string, logger *logging.EventLogger) error {
	// Try to extract Tier 1 count from table output
	// Pattern: "Tier 1 (Blocking): X" or similar
	tier1Pattern := regexp.MustCompile(`Tier\s+1.*?Blocking.*?:\s*(\d+)`)
	matches := tier1Pattern.FindStringSubmatch(output)

	tier1Count := 0
	if len(matches) > 1 {
		fmt.Sscanf(matches[1], "%d", &tier1Count)
	}

	if tier1Count > 0 {
		err := errfmt.Errorf("tier 1 (blocking) violations detected: %d. resolve violations before committing (POLICY-CODE-004, POLICY-CODE-005)", tier1Count)
		logging.FluentEvent(logger).Error("Tier 1 (Blocking) violations detected", err).
			Int("count", tier1Count).
			Log()
		logging.FluentEvent(logger).Info("POLICY-CODE-004: All system check violations must be resolved before committing").Log()
		logging.FluentEvent(logger).Info("POLICY-CODE-005: Zero tolerance for Tier 1 violations").Log()
		logging.FluentEvent(logger).Info("").Log()
		logging.FluentEvent(logger).Info("To view details:").Log()
		logging.FluentEvent(logger).Info(fmt.Sprintf("  %s system check --verbose", paths.CLICommandName)).Log()
		logging.FluentEvent(logger).Info("").Log()
		logging.FluentEvent(logger).Info("To resolve:").Log()
		logging.FluentEvent(logger).Info(fmt.Sprintf("  %s system check --auto-fix --force", paths.CLICommandName)).Log()
		return err
	}

	logging.FluentEvent(logger).Info("System health check passed (no Tier 1 violations detected)").Log()
	return nil
}

// detectGitHostingPlatform detects whether the repository is hosted on GitHub or GitLab
func detectGitHostingPlatform(projectRoot string) (string, error) {
	gf := scsfacade.New(projectRoot)
	remoteURL, err := gf.OriginURL()
	if err != nil {
		return "", errfmt.Newf("failed to get remote URL").Wrap(err)
	}

	// Check for GitHub
	if strings.Contains(remoteURL, "github.com") {
		return "github", nil
	}

	// Check for GitLab
	if strings.Contains(remoteURL, "gitlab.com") || strings.Contains(remoteURL, "gitlab") {
		return "gitlab", nil
	}

	return "", errfmt.Errorf("unknown Git hosting platform (expected GitHub or GitLab): %s", remoteURL)
}

// createPullRequest creates a PR (GitHub) or MR (GitLab) per POLICY-WORKFLOW-002
func createPullRequest(projectRoot string, storageProvider storage.ObjectStorageProvider, cmdCtx context.Context, currentPlan map[string]any, branchName string, prTitle, prBody string, logger *logging.EventLogger, dryRun bool) error {
	if dryRun {
		logging.FluentEvent(logger).Info(cli.DryRunWould("create Pull Request/Merge Request (POLICY-WORKFLOW-002)")).Log()
		return nil
	}

	// Detect Git hosting platform
	platform, err := detectGitHostingPlatform(projectRoot)
	if err != nil {
		return errfmt.Newf("failed to detect Git hosting platform").Wrap(err)
	}

	// Get current branch if not provided
	if branchName == emptyValue {
		branchName, err = getCurrentBranch(projectRoot)
		if err != nil {
			return err
		}
	}

	// Prepare PR title and body
	prTitle, prBody, err = preparePRTitleAndBody(prTitle, prBody, storageProvider, cmdCtx, currentPlan, branchName, logger)
	if err != nil {
		return err
	}

	// Verify backlog items completion (POLICY-WORKFLOW-002 requirement)
	verifyBacklogItemsCompletion(currentPlan, storageProvider, cmdCtx, logger)

	// Create PR/MR using appropriate CLI
	return createPRForPlatform(platform, projectRoot, branchName, prTitle, prBody, logger)
}

// generatePRDetails generates PR title and body from priority plan
func generatePRDetails(storageProvider storage.ObjectStorageProvider, cmdCtx context.Context, currentPlan map[string]any, _ *logging.EventLogger) (title, body string, err error) {
	if currentPlan == nil {
		return "", "", errfmt.Errorf("no priority plan available")
	}

	planID, _ := currentPlan[objects.FieldKeyID].(string)
	planTitle, _ := currentPlan[objects.FieldKeyTitle].(string)
	planDescription, _ := currentPlan[objects.FieldKeyDescription].(string)
	planContext, _ := currentPlan[objects.FieldKeyContext].(string)

	// Generate title
	title = fmt.Sprintf("%s: %s", planID, planTitle)

	// Generate body
	var bodyBuilder strings.Builder
	fmt.Fprintf(&bodyBuilder, "## %s\n\n", planTitle)

	if planDescription != emptyValue {
		fmt.Fprintf(&bodyBuilder, "**Description:**\n%s\n\n", planDescription)
	}

	if planContext != emptyValue {
		fmt.Fprintf(&bodyBuilder, "**Context:**\n%s\n\n", planContext)
	}

	// Add backlog items summary
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{
		Kind:    objects.KindBacklogItem,
		Filters: map[string]any{objects.FieldKeyPriorityPlanRef: planID},
	}

	result, err := storageProvider.List(cmdCtx, secCtx, storageCtx, filter)
	if err == nil && len(result.Objects) > 0 {
		bodyBuilder.WriteString("## Backlog Items\n\n")
		completeCount := 0
		for _, item := range result.Objects {
			status, _ := item[objects.FieldKeyStatus].(string)
			if status == backlogStatusComplete {
				completeCount++
			}
		}
		fmt.Fprintf(&bodyBuilder, "- Total items: %d\n", len(result.Objects))
		fmt.Fprintf(&bodyBuilder, "- Completed: %d\n", completeCount)
		fmt.Fprintf(&bodyBuilder, "- Remaining: %d\n\n", len(result.Objects)-completeCount)
	}

	bodyBuilder.WriteString("---\n\n")
	fmt.Fprintf(&bodyBuilder, "*This PR was automatically created via `%s system sync --create-pr` (POLICY-WORKFLOW-002)*\n", paths.CLICommandName)

	body = bodyBuilder.String()
	return title, body, nil
}

// checkAllBacklogItemsComplete verifies all backlog items for a priority plan are complete
func checkAllBacklogItemsComplete(storageProvider storage.ObjectStorageProvider, cmdCtx context.Context, planID string, _ *logging.EventLogger) (bool, error) {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{
		Kind:    objects.KindBacklogItem,
		Filters: map[string]any{objects.FieldKeyPriorityPlanRef: planID},
	}

	result, err := storageProvider.List(cmdCtx, secCtx, storageCtx, filter)
	if err != nil {
		return false, err
	}

	if len(result.Objects) == 0 {
		return true, nil // No items means "all complete" (trivially)
	}

	for _, item := range result.Objects {
		status, _ := item[objects.FieldKeyStatus].(string)
		if status != backlogStatusComplete && status != objects.ObjectStatusArchived {
			return false, nil
		}
	}

	return true, nil
}

// createGitHubPR creates a PR using GitHub CLI (gh)
func createGitHubPR(projectRoot, branchName, title, body string, logger *logging.EventLogger) error {
	// Check if gh CLI is available
	if _, err := exec.LookPath("gh"); err != nil {
		return errfmt.Errorf("GitHub CLI (gh) not found. Install from https://cli.github.com/")
	}

	logging.FluentEvent(logger).Info("Creating GitHub Pull Request...").
		String("branch", branchName).
		String("title", title).
		Log()

	// Create PR using gh CLI
	cmd := exec.Command("gh", "pr", "create",
		"--title", title,
		"--body", body,
		"--head", branchName)
	zqkenv.WireExecForIsolatedProject(cmd, projectRoot)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return errfmt.Errorf("failed to create GitHub PR: %s", string(output))
	}

	logging.FluentEvent(logger).Info("GitHub Pull Request created successfully").
		String("output", strings.TrimSpace(string(output))).
		Log()
	return nil
}

// createGitLabMR creates a MR using GitLab CLI (glab)
func createGitLabMR(projectRoot, branchName, title, body string, logger *logging.EventLogger) error {
	// Check if glab CLI is available
	if _, err := exec.LookPath("glab"); err != nil {
		return errfmt.Errorf("GitLab CLI (glab) not found. Install from https://gitlab.com/gitlab-org/cli")
	}

	logging.FluentEvent(logger).Info("Creating GitLab Merge Request...").
		String("branch", branchName).
		String("title", title).
		Log()

	// Create MR using glab CLI
	cmd := exec.Command("glab", "mr", "create",
		"--title", title,
		"--description", body,
		"--source-branch", branchName)
	zqkenv.WireExecForIsolatedProject(cmd, projectRoot)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return errfmt.Errorf("failed to create GitLab MR: %s", string(output))
	}

	logging.FluentEvent(logger).Info("GitLab Merge Request created successfully").
		String("output", strings.TrimSpace(string(output))).
		Log()
	return nil
}
