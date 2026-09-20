package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentprompt"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewNextCmd creates the next command for agent tasks
func NewNextCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentNextCommandBuilder()
	cmd.Use = "next [task_id]"
	cmd.Args = cobra.ExactArgs(1)
	cmd.RunE = cli.WithProcessor(runNext)
	return cmd
}

func runNext(cmd *cobra.Command, args []string, proc *cli.Processor) error {
	ctx := proc.OperationContext()
	id := args[0]
	secCtx := proc.SecurityContext()
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}

	// Read the current object to ensure it exists
	task, err := proc.Storage().Read(ctx, secCtx, id)
	if err != nil {
		return errfmt.Newf("failed to read task %s", id).Wrap(err)
	}

	// Force transition to pending_verification (system context overrides r--)
	updates := map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusPendingVerification,
	}

	// Inject a default assignee_persona_ref if missing, so validation passes
	if task[objects.FieldKeyAssigneePersonaRef] == nil || task[objects.FieldKeyAssigneePersonaRef] == "" {
		updates[objects.FieldKeyAssigneePersonaRef] = objects.ConstPersonaOrchestratorAlpha
	}

	title, _ := task[objects.FieldKeyTitle].(string)
	desc, _ := task[objects.FieldKeyDescription].(string)
	workClass := agentprompt.ClassifyWorkClass(title, desc)
	if err := verifyAgentNextEvidence(ctx, proc.ProjectRoot(), id, task, workClass); err != nil {
		return err
	}

	err = proc.Storage().Update(ctx, secCtx, id, updates)
	if err != nil {
		return errfmt.Newf("failed to auto-transition state for %s", id).Wrap(err)
	}

	// Release work claim after worker advances.
	_ = releaseTaskAfterNext(proc, id, "")

	// Emit verification_signal to health.jsonl
	testPattern := ""
	if tp, ok := task["test_pattern"].(string); ok {
		testPattern = tp
	} else if tc, ok := task["test_command"].(string); ok {
		testPattern = tc
	} else {
		testPattern = fmt.Sprintf("heuristic-task-%s", id)
	}

	verificationEvent := map[string]any{
		"timestamp":               time.Now().Format(time.RFC3339),
		objects.FieldKeyEventType: "verification_signal",
		"task_id":                 id,
		"test_pattern":            testPattern,
		objects.FieldKeyStatus:    objects.ObjectStatusPendingVerification,
		"result":                  objects.ObjectStatusPendingVerification,
	}
	schedulerpkg.AppendTestBundleHealthEvent(proc.ProjectRoot(), id, verificationEvent)

	_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Successfully transitioned task %s to status: pending_verification\n", id)))

	// Workers must register wake-on-validation-failure.
	// Default to wake so omitting the flag is not fire-and-forget.
	// none/off is refused — no env privilege override.
	onValidationFailure, _ := cmd.Flags().GetString("on-validation-failure")
	onVF := strings.TrimSpace(onValidationFailure)
	if onVF == "" {
		onVF = "wake"
	}
	if strings.EqualFold(onVF, "none") || strings.EqualFold(onVF, "off") {
		return errfmt.Errorf(
			"agent next requires --on-validation-failure wake (none/off refused; no env override)",
		)
	}
	if onVF != "" {
		// Register a callback via a scheduler_job
		jobID := fmt.Sprintf("SCH-%s-cb-%d", id, time.Now().Unix())
		job := map[string]any{
			objects.FieldKeyID:               jobID,
			objects.FieldKeyKind:             objects.KindSchedulerJob,
			objects.FieldKeyStatus:           objects.ObjectStatusPending,
			objects.FieldKeyTitle:            fmt.Sprintf("Wake agent for %s verification failure", id),
			objects.FieldKeyCallbackOnStatus: objects.ObjectStatusError, // Wake on error/failure
			"target_ref":                     id,
			"action":                         onVF,
		}

		// Create the scheduler job outside the membrane if needed
		err = proc.Storage().Create(ctx, secCtx, job)
		if err != nil {
			_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Warning: failed to register validation failure callback: %v\n", err)))
		} else {
			_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Registered wake callback %s for validation failures.\n", jobID)))
		}
	}

	return nil
}

func verifyAgentNextEvidence(ctx context.Context, projectRoot, taskID string, task map[string]any, workClass agentprompt.WorkClass) error {
	if workClass.IsDocsEval() {
		if _, err := requirePriorityPlanRef(task, taskID); err != nil {
			return err
		}
		desc, _ := task[objects.FieldKeyDescription].(string)
		return verifyDocsEvalNextEvidence(projectRoot, desc)
	}
	if err := requireAtkWorktreeTornDown(projectRoot, taskID); err != nil {
		return err
	}
	priRef, err := requirePriorityPlanRef(task, taskID)
	if err != nil {
		return err
	}
	return requireCommitsMergedToIntegration(ctx, projectRoot, taskID, priRef, task)
}

func requirePriorityPlanRef(task map[string]any, taskID string) (string, error) {
	priRef, ok := task[objects.FieldKeyPriorityPlanRef].(string)
	if !ok || priRef == "" {
		return "", errfmt.Errorf("FAIL-CLOSED: priority_plan_ref is missing on task %s. You must link your work to a priority plan and merge to its integration branch.", taskID)
	}
	return priRef, nil
}

func requireAtkWorktreeTornDown(projectRoot, taskID string) error {
	for _, worktreeDir := range paths.AgentWorktreeLookupDirs(projectRoot, taskID) {
		if _, statErr := fileutil.Stat(worktreeDir); statErr == nil {
			return errfmt.Errorf("FAIL-CLOSED: worktree %s still exists. You must merge your work into the active integration branch (integration/pri-*) and tear down the worktree before calling 'agent next'.", worktreeDir)
		}
	}
	return nil
}

func requireCommitsMergedToIntegration(ctx context.Context, projectRoot, taskID, priRef string, task map[string]any) error {
	commitRefs, _ := task[objects.FieldKeyCommitHashes].([]any)
	if len(commitRefs) == 0 {
		return errfmt.Errorf("FAIL-CLOSED: no commit_hashes found on task %s. You must merge your work into integration/%s and update commit_hashes before calling 'agent next'.", taskID, strings.ToLower(priRef))
	}
	integrationBranch := "integration/" + strings.ToLower(priRef)
	for _, cr := range commitRefs {
		crStr, ok := cr.(string)
		if !ok || crStr == "" {
			continue
		}
		cmd := execwrap.CommandContext(ctx, "git", "branch", "--contains", crStr)
		cmd.Dir = projectRoot
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(out), integrationBranch) {
			return errfmt.Errorf("FAIL-CLOSED: commit %s is not merged into %s. You must merge your work before calling 'agent next'.", crStr, integrationBranch)
		}
	}
	return nil
}

var docsEvalFindingsPathRe = regexp.MustCompile(`docs/quality/cef-runs/[^\s]+/findings/[A-Za-z0-9_.-]+\.jsonl`)

// verifyDocsEvalNextEvidence enforces SUCCESS_GATE-shaped proof for CEF/docs ATKs:
// at least one findings JSONL path from the description exists with a real finding_id.
func verifyDocsEvalNextEvidence(projectRoot, description string) error {
	pathsFound := docsEvalFindingsPathRe.FindAllString(description, -1)
	if len(pathsFound) == 0 {
		return errfmt.Errorf("FAIL-CLOSED: docs_eval ATK description has no docs/quality/cef-runs/.../findings/*.jsonl path to verify")
	}
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = "."
	}
	var lastErr error
	for _, rel := range pathsFound {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		data, err := fileutil.ReadFile(abs)
		if err != nil {
			lastErr = errfmt.Newf("FAIL-CLOSED: docs_eval evidence missing %s", rel).Wrap(err)
			continue
		}
		n := 0
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.Contains(line, "REPLACE_ME") {
				continue
			}
			var obj map[string]any
			if json.Unmarshal([]byte(line), &obj) != nil {
				continue
			}
			fid, _ := obj["finding_id"].(string)
			if strings.TrimSpace(fid) != "" {
				n++
			}
		}
		if n >= 1 {
			return nil
		}
		lastErr = errfmt.Errorf("FAIL-CLOSED: docs_eval SUCCESS_GATE failed for %s (need >=1 finding_id)", rel)
	}
	if lastErr != nil {
		return lastErr
	}
	return errfmt.Errorf("FAIL-CLOSED: docs_eval SUCCESS_GATE failed")
}
