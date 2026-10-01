package system

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/objects"
)

// SyncConfig holds all sync operation configuration
type SyncConfig struct {
	Pull        bool
	Push        bool
	Verify      bool
	Stage       bool
	Commit      bool
	AutoCommit  bool
	Message     string
	CheckHealth bool
	CreatePR    bool
	PRTitle     string
	PRBody      string
	DryRun      bool
}

// validateGitRepository validates that the project root is a git repository
func validateGitRepository(projectRoot string, logger *logging.EventLogger) error {
	gitDir := filepath.Join(projectRoot, ".git")
	if _, err := fileutil.Stat(gitDir); err != nil {
		err := errfmt.Errorf("not a git repository")
		logging.FluentEvent(logger).Error("Git repository check failed", err).Log()
		return err
	}
	return nil
}

// handlePriorityPlanAndBranch handles priority plan lookup and branch management
func handlePriorityPlanAndBranch(processor *cli.Processor, projectRoot string, autoCommit bool, message string, logger *logging.EventLogger, dryRun bool) (map[string]any, error) {
	currentPlan, err := findCurrentPriorityPlan(processor.OperationContext(), processor.Storage())
	if err != nil {
		return handlePriorityPlanError(err, autoCommit, message, logger)
	}

	// Ensure we're on the correct branch
	if err := ensureBranchForPlan(currentPlan, projectRoot, logger, dryRun); err != nil {
		return nil, err
	}

	return currentPlan, nil
}

// handlePriorityPlanError handles errors when finding priority plan
func handlePriorityPlanError(err error, autoCommit bool, message string, logger *logging.EventLogger) (map[string]any, error) {
	if strings.Contains(err.Error(), "no active or in_progress priority plans found") {
		logging.FluentEvent(logger).Warn("No active priority plan found. Branch management will be skipped.").
			String("suggestion", "Create a priority plan or use --message for commit messages").
			Log()

		if autoCommit && message == emptyValue {
			err := errfmt.Errorf("cannot auto-generate commit message without active priority plan. Use --message to provide a commit message")
			logging.FluentEvent(logger).Error("Auto-commit validation failed", err).Log()
			return nil, err
		}
		return nil, nil // Continue without plan
	}

	// Other error (e.g., storage error)
	logging.FluentEvent(logger).Warn("Could not determine current priority plan, continuing with current branch").
		WithError(err).
		Log()
	return nil, nil
}

// ensureBranchForPlan ensures we're on the correct branch for a priority plan
func ensureBranchForPlan(currentPlan map[string]any, projectRoot string, logger *logging.EventLogger, dryRun bool) error {
	planID, _ := currentPlan[objects.FieldKeyID].(string)
	if planID == emptyValue {
		return nil
	}

	expectedBranch := generateBranchName(planID)
	if err := ensureBranch(projectRoot, expectedBranch, logger, dryRun); err != nil {
		logErr := errfmt.Newf("failed to ensure branch").Wrap(err)
		logging.FluentEvent(logger).Error("Branch management failed", logErr).
			String("branch", expectedBranch).
			Log()
		return logErr
	}
	return nil
}

// handleHealthCheck handles system health check if needed
func handleHealthCheck(checkHealth bool, commit bool, autoCommit bool, push bool, projectRoot string, logger *logging.EventLogger, dryRun bool) error {
	if !checkHealth || (!commit && !autoCommit && !push) {
		return nil
	}

	if err := runSystemHealthCheck(projectRoot, logger, dryRun); err != nil {
		return err
	}
	return nil
}

// handleStaging handles git staging if requested
func handleStaging(stage bool, cmd *cobra.Command, projectRoot string, logger *logging.EventLogger, dryRun bool) error {
	if !stage {
		return nil
	}

	if err := gitStage(cmd, projectRoot, logger, dryRun); err != nil {
		logErr := errfmt.Newf("failed to stage changes").Wrap(err)
		logging.FluentEvent(logger).Error("Git stage failed", logErr).Log()
		return logErr
	}
	return nil
}

// handleCommitting handles git committing if requested
func handleCommitting(commit bool, autoCommit bool, message string, cmd *cobra.Command, projectRoot string, storageProvider storage.ObjectStorageProvider, processor *cli.Processor, currentPlan map[string]any, logger *logging.EventLogger, dryRun bool) error {
	if !commit && !autoCommit {
		return nil
	}

	// Generate commit message if auto-commit
	finalMessage, err := prepareCommitMessage(autoCommit, message, currentPlan, projectRoot, storageProvider, processor, logger)
	if err != nil {
		return err
	}

	// Validate message is not empty
	if finalMessage == emptyValue {
		err := errfmt.Errorf("commit message required (use --message or --auto-commit)")
		logging.FluentEvent(logger).Error("Commit validation failed", err).Log()
		return err
	}

	// Perform commit
	if err := gitCommit(cmd, projectRoot, finalMessage, logger, dryRun); err != nil {
		logErr := errfmt.Newf("failed to commit").Wrap(err)
		logging.FluentEvent(logger).Error("Git commit failed", logErr).Log()
		return logErr
	}

	return nil
}

// prepareCommitMessage prepares the commit message (generates if auto-commit, otherwise uses provided)
func prepareCommitMessage(autoCommit bool, message string, currentPlan map[string]any, projectRoot string, storageProvider storage.ObjectStorageProvider, processor *cli.Processor, logger *logging.EventLogger) (string, error) {
	if !autoCommit {
		return message, nil
	}

	if currentPlan == nil {
		err := errfmt.Errorf("cannot auto-generate commit message without active priority plan. Use --message to provide a commit message")
		logging.FluentEvent(logger).Error("Auto-commit validation failed", err).Log()
		return "", err
	}

	generatedMsg, err := generateCommitMessage(projectRoot, storageProvider, processor.OperationContext(), currentPlan, logger)
	if err != nil {
		logErr := errfmt.Newf("failed to generate commit message").Wrap(err)
		logging.FluentEvent(logger).Error("Commit message generation failed", logErr).Log()
		return "", logErr
	}

	return generatedMsg, nil
}

// handlePulling handles git pull if requested
func handlePulling(pull bool, projectRoot string, verify bool, logger *logging.EventLogger, dryRun bool) error {
	if !pull {
		return nil
	}

	if err := gitPull(projectRoot, verify, logger, dryRun); err != nil {
		logErr := errfmt.Newf("failed to pull").Wrap(err)
		logging.FluentEvent(logger).Error("Git pull failed", logErr).Log()
		return logErr
	}
	return nil
}

// handlePushing handles git push and PR creation if requested
func handlePushing(push bool, createPR bool, cmd *cobra.Command, projectRoot string, verify bool, storageProvider storage.ObjectStorageProvider, processor *cli.Processor, currentPlan map[string]any, prTitle string, prBody string, logger *logging.EventLogger, dryRun bool) error {
	if !push {
		return nil
	}

	if err := gitPush(cmd, projectRoot, verify, logger, dryRun); err != nil {
		logErr := errfmt.Newf("failed to push").Wrap(err)
		logging.FluentEvent(logger).Error("Git push failed", logErr).Log()
		return logErr
	}

	// Create PR/MR if requested
	if createPR {
		if err := createPullRequest(projectRoot, storageProvider, processor.OperationContext(), currentPlan, "", prTitle, prBody, logger, dryRun); err != nil {
			logErr := errfmt.Newf("failed to create PR/MR").Wrap(err)
			logging.FluentEvent(logger).Error("PR/MR creation failed", logErr).Log()
			return logErr
		}
	}

	return nil
}

// getChangedFiles gets the list of changed files from git status
func getChangedFiles(projectRoot string) ([]string, error) {
	statusCmd := execwrap.Command("git", "status", "--short")
	zqkenv.WireExecForIsolatedProject(statusCmd, projectRoot)
	statusOutput, err := statusCmd.Output()
	if err != nil {
		return nil, errfmt.Newf("failed to get git status").Wrap(err)
	}

	changedFiles := strings.Split(strings.TrimSpace(string(statusOutput)), "\n")
	if len(changedFiles) == 0 || (len(changedFiles) == 1 && changedFiles[0] == emptyValue) {
		return nil, errfmt.Errorf("no changes to commit")
	}
	return changedFiles, nil
}

// extractBacklogItems extracts backlog item IDs from changed files
func extractBacklogItems(changedFiles []string) map[string]bool {
	backlogItemPattern := regexp.MustCompile(`BLI-\d+`)
	backlogItems := make(map[string]bool)

	for _, file := range changedFiles {
		matches := backlogItemPattern.FindAllString(file, -1)
		for _, match := range matches {
			backlogItems[match] = true
		}
	}
	return backlogItems
}

// buildPriorityPlanMessage builds the priority plan part of the commit message
func buildPriorityPlanMessage(currentPlan map[string]any) string {
	if currentPlan == nil {
		return ""
	}

	planID, ok := currentPlan[objects.FieldKeyID].(string)
	if !ok {
		return ""
	}

	title, ok := currentPlan[objects.FieldKeyTitle].(string)
	if ok {
		return fmt.Sprintf("%s: %s", planID, title)
	}
	return planID
}

// buildBacklogItemsMessage builds the backlog items part of the commit message
func buildBacklogItemsMessage(backlogItems map[string]bool) string {
	if len(backlogItems) == 0 {
		return ""
	}

	itemList := make([]string, 0, len(backlogItems))
	for item := range backlogItems {
		itemList = append(itemList, item)
	}

	// Sort for consistency
	for i := 0; i < len(itemList)-1; i++ {
		for j := i + 1; j < len(itemList); j++ {
			if itemList[i] > itemList[j] {
				itemList[i], itemList[j] = itemList[j], itemList[i]
			}
		}
	}

	return "\n\nRelated backlog items: " + strings.Join(itemList, ", ")
}

// buildCommitMessageParts assembles all parts of the commit message
func buildCommitMessageParts(currentPlan map[string]any, backlogItems map[string]bool, fileCount int) string {
	var msgParts []string

	// Add priority plan reference
	if planMsg := buildPriorityPlanMessage(currentPlan); planMsg != emptyValue {
		msgParts = append(msgParts, planMsg)
	}

	// Add backlog item references
	if itemsMsg := buildBacklogItemsMessage(backlogItems); itemsMsg != emptyValue {
		msgParts = append(msgParts, itemsMsg)
	}

	// Add summary of changes
	if fileCount > 0 {
		msgParts = append(msgParts, fmt.Sprintf("\n\nChanged %d file(s)", fileCount))
	}

	message := strings.Join(msgParts, "")
	if message == emptyValue {
		message = fmt.Sprintf("Update: %d file(s) changed", fileCount)
	}

	return message
}

// getCurrentBranch gets the current git branch name
func getCurrentBranch(projectRoot string) (string, error) {
	branchCmd := execwrap.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	zqkenv.WireExecForIsolatedProject(branchCmd, projectRoot)
	branchOutput, err := branchCmd.Output()
	if err != nil {
		return "", errfmt.Newf("failed to determine current branch").Wrap(err)
	}
	return strings.TrimSpace(string(branchOutput)), nil
}

// preparePRTitleAndBody prepares PR title and body, generating from priority plan if needed
func preparePRTitleAndBody(prTitle, prBody string, storageProvider storage.ObjectStorageProvider, cmdCtx context.Context, currentPlan map[string]any, branchName string, logger *logging.EventLogger) (string, string, error) {
	if prTitle != emptyValue && prBody != emptyValue {
		return prTitle, prBody, nil
	}

	title, body, err := generatePRDetails(storageProvider, cmdCtx, currentPlan, logger)
	if err != nil {
		logging.FluentEvent(logger).Warn("Failed to generate PR details from priority plan").
			WithError(err).
			Log()
		if prTitle == emptyValue {
			prTitle = fmt.Sprintf("Update: %s", branchName)
		}
		if prBody == emptyValue {
			prBody = "Automated PR/MR creation"
		}
		return prTitle, prBody, nil
	}

	if prTitle == emptyValue {
		prTitle = title
	}
	if prBody == emptyValue {
		prBody = body
	}

	return prTitle, prBody, nil
}

// verifyBacklogItemsCompletion verifies all backlog items are complete for a priority plan
func verifyBacklogItemsCompletion(currentPlan map[string]any, storageProvider storage.ObjectStorageProvider, cmdCtx context.Context, logger *logging.EventLogger) {
	if currentPlan == nil {
		return
	}

	planID, _ := currentPlan[objects.FieldKeyID].(string)
	if planID == emptyValue {
		return
	}

	allComplete, err := checkAllBacklogItemsComplete(storageProvider, cmdCtx, planID, logger)
	if err != nil {
		logging.FluentEvent(logger).Warn("Failed to verify backlog item completion").
			WithError(err).
			Log()
		return
	}

	if !allComplete {
		logging.FluentEvent(logger).Warn("Not all backlog items for priority plan are complete").
			String("plan_id", planID).
			String("suggestion", "Complete all backlog items before creating PR (POL-WORKFLOW-002)").
			Log()
	}
}

// createPRForPlatform creates PR/MR using the appropriate platform CLI
func createPRForPlatform(platform, projectRoot, branchName, prTitle, prBody string, logger *logging.EventLogger) error {
	switch platform {
	case "github":
		return createGitHubPR(projectRoot, branchName, prTitle, prBody, logger)
	case "gitlab":
		return createGitLabMR(projectRoot, branchName, prTitle, prBody, logger)
	default:
		return errfmt.Errorf("unsupported platform: %s", platform)
	}
}
