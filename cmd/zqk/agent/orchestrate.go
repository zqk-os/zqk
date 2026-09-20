package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentdelivery"
	"github.com/zqk-os/zqk/pkg/agentprompt"
	"github.com/zqk-os/zqk/pkg/authcred"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

var (
	// TestDelivererOverride allows integration tests to mock agent delivery.
	TestDelivererOverride agentdelivery.Deliverer
)

// OrchestrateOptions holds the execution options for orchestrate
type OrchestrateOptions struct {
	SessionID      string
	AmbientContext string
	PersonaID      string
	Timeout        time.Duration
}

// NewOrchestrateCmd creates the orchestrate command
func NewOrchestrateCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentOrchestrateCommandBuilder()
	cmd.Flags().String("persona-id", "", "Optional persona ID to filter work and define agent role")
	if cmd.Flags().Lookup(cli.FlagTimeout) == nil {
		cmd.Flags().Duration(cli.FlagTimeout, 0, "Timeout for orchestration session (e.g. 4h, 30m; 0 = default 4h)")
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		sessionID, _ := cmd.Flags().GetString("session-id")
		ambientContext, _ := cmd.Flags().GetString("ambient-context")
		personaID, _ := cmd.Flags().GetString("persona-id")
		timeout := cli.GetTimeout(cmd)
		opts := OrchestrateOptions{
			SessionID:      sessionID,
			AmbientContext: ambientContext,
			PersonaID:      personaID,
			Timeout:        timeout,
		}
		var planArg string
		if len(args) > 0 {
			planArg = args[0]
		}
		return runOrchestrate(cmd, planArg, opts)
	}

	return cmd
}

const (
	defaultHitlConfidence     = 0.8
	defaultHitlDecisionBranch = "branch-main"
	hitlPolicyID              = "POL-HITL-001"
	orchestratedTaskStatus    = objects.ObjectStatusApproved
	taskReadinessAttempts     = 20
	taskReadinessDelay        = 50 * time.Millisecond
	orchestrationTimeout      = 4 * time.Hour
	nativeSwarmConcurrency    = 2
)

func resolveOrchestrationTimeout(optsTimeout time.Duration, cmd *cobra.Command) time.Duration {
	if optsTimeout > 0 {
		return optsTimeout
	}
	if cmd != nil {
		if cmdTimeout := cli.GetTimeout(cmd); cmdTimeout > 0 {
			return cmdTimeout
		}
	}
	return orchestrationTimeout
}

func backlogItemEligibleForOrchestration(status string) bool {
	switch status {
	case objects.ObjectStatusComplete,
		objects.ObjectStatusArchived,
		objects.ObjectStatusCancelled,
		objects.ObjectStatusImplemented,
		objects.ObjectStatusPendingVerification:
		return false
	default:
		return true
	}
}

func waitForAgentTaskReadable(
	ctx context.Context,
	sp storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	taskID string,
) error {
	var lastErr error
	for range taskReadinessAttempts {
		obj, err := sp.Read(ctx, secCtx, taskID)
		if err == nil && obj != nil {
			return nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = storage.ErrObjectNotFound
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(taskReadinessDelay):
		}
	}
	return errfmt.Errorf("agent task %s not readable after create: %v", taskID, lastErr)
}

func orchestrationTaskOpts(
	state *orchestratorState,
	targetAgent, personaID, capability, taskTitle string,
) agentprompt.TaskPromptOptions {
	workClass := agentprompt.ClassifyWorkClass(taskTitle, state.ambientSection)
	includeTDD := true
	includeObserver := true
	if workClass.IsDocsEval() {
		includeTDD = false
		includeObserver = false
	}
	return agentprompt.TaskPromptOptions{
		PlanTitle:       state.title,
		PlanID:          state.planID,
		SessionID:       state.opts.SessionID,
		TargetAgent:     targetAgent,
		PersonaID:       personaID,
		Capability:      capability,
		TaskTitle:       taskTitle,
		IncludeTDD:      includeTDD,
		IncludeObserver: includeObserver,
		TaskContext:     state.ambientSection,
	}
}

func buildOrchestrationTaskPrompt(
	ctx context.Context,
	state *orchestratorState,
	targetAgent, personaID, capability, taskTitle, meshSkillSection string,
) string {
	opts := orchestrationTaskOpts(state, targetAgent, personaID, capability, taskTitle)
	promptMarkdown, err := agentprompt.BuildTaskPrompt(
		ctx,
		state.sp,
		state.secCtx,
		state.proc.ProjectRoot(),
		opts,
	)
	if err != nil {
		promptMarkdown = fmt.Sprintf("Task: %s", taskTitle)
	}
	return promptMarkdown + meshSkillSection
}

func buildOrchestrationTaskEnvelope(
	ctx context.Context,
	state *orchestratorState,
	targetAgent, personaID, capability, taskTitle, meshSkillSection string,
) *agentprompt.TaskEnvelope {
	opts := orchestrationTaskOpts(state, targetAgent, personaID, capability, taskTitle)
	opts.Layer = agentprompt.PromptLayerPersist
	// Ambient file/text is execute-time context; do not copy it onto every ATK.
	if state.opts.AmbientContext != "" {
		opts.TaskContext = "Ambient context supplied at orchestrate; resolve at execute / prepare-context."
	} else {
		opts.TaskContext = ""
	}
	env, err := agentprompt.BuildTaskEnvelope(
		ctx,
		state.sp,
		state.secCtx,
		state.proc.ProjectRoot(),
		opts,
	)
	if err != nil || env == nil {
		env = &agentprompt.TaskEnvelope{
			Description: fmt.Sprintf("Task: %s\n%s\n", taskTitle, agentprompt.TaskEnvelopeMarker),
			PolicyRefs:  agentprompt.StandingPolicyRefs(),
		}
	}
	if meshSkillSection != "" {
		env.Description += meshSkillSection
	}
	return env
}

type orchestratorState struct {
	cmd             *cobra.Command
	proc            *cli.Processor
	secCtx          *pkgctx.SecurityContext
	sp              storage.ObjectStorageProvider
	opts            OrchestrateOptions
	planArg         string
	planID          string
	title           string
	items           []map[string]any
	personaRole     string
	policySection   string
	feedbackSection string
	ambientSection  string
	processedItems  []string
	routingPlan     map[string]any
	isPipeline      bool
	isStrategicPlan bool
}

//nolint:gocyclo
func hasAnyPersonaAssignment(obj map[string]any) bool {
	if obj == nil {
		return false
	}
	if refsAny := obj[objects.FieldKeyPersonaRefs]; refsAny != nil {
		switch refs := refsAny.(type) {
		case []any:
			for _, rAny := range refs {
				if r, ok := rAny.(string); ok && strings.TrimSpace(r) != "" {
					return true
				}
			}
		case []string:
			for _, r := range refs {
				if strings.TrimSpace(r) != "" {
					return true
				}
			}
		case string:
			if strings.TrimSpace(refs) != "" {
				return true
			}
		}
	}
	if assigneeAny := obj[objects.FieldKeyAssigneePersonaRef]; assigneeAny != nil {
		if assignee, ok := assigneeAny.(string); ok && strings.TrimSpace(assignee) != "" {
			return true
		}
	}
	return false
}

func hasPersonaMatch(obj map[string]any, personaIDs []string) bool {
	if len(personaIDs) == 0 {
		return true // no filtering
	}
	// Unassigned work stays eligible under --persona-id (soft match). Hard-drop only when the
	// item explicitly names other personas.
	if !hasAnyPersonaAssignment(obj) {
		return true
	}
	refsAny := obj[objects.FieldKeyPersonaRefs]
	if refsAny != nil {
		switch refs := refsAny.(type) {
		case []any:
			for _, rAny := range refs {
				if r, okStr := rAny.(string); okStr {
					for _, pid := range personaIDs {
						if strings.EqualFold(r, pid) {
							return true
						}
					}
				}
			}
		case []string:
			for _, r := range refs {
				for _, pid := range personaIDs {
					if strings.EqualFold(r, pid) {
						return true
					}
				}
			}
		}
	}

	// Support assignee_persona_ref for agent_task
	assigneeAny := obj[objects.FieldKeyAssigneePersonaRef]
	if assigneeAny != nil {
		if assignee, ok := assigneeAny.(string); ok {
			for _, pid := range personaIDs {
				if strings.EqualFold(assignee, pid) {
					return true
				}
			}
		}
	}

	return false
}

// nativeSwarmEligible reports whether model_tier should spawn local sync-loop
// workers (Ollama / native swarm) instead of parking on primary/vendor claim.
func nativeSwarmEligible(modelTier string) bool {
	switch strings.ToLower(strings.TrimSpace(modelTier)) {
	case "tier_1_routine", "tier_2_simple", "tier_3_light", "tier_3_simple":
		return true
	default:
		// tier_1_complex and unknown → primary/human-grade seats
		return false
	}
}

// seedAgentWorktreeRuntime makes ignored runtime membranes available inside an isolated git
// worktree. Tracked process YAML already comes from git; hidden CAS indexes do not.
// The draft plane is an isolated empty directory — never a symlink to studio object_drafts.
// The CAP coordinator owns the ATK in the main root, so no task CAS blob is copied.
func seedAgentWorktreeRuntime(mainRoot, worktreeRoot string) error {
	mainProcessDir := filepath.Join(mainRoot, paths.ProcessDir)
	kindEntries, err := fileutil.ReadDir(mainProcessDir)
	if err != nil {
		return errfmt.Newf("read process directory").Wrap(err)
	}
	for _, kindEntry := range kindEntries {
		if !kindEntry.IsDir() {
			continue
		}
		srcDir := filepath.Join(mainProcessDir, kindEntry.Name())
		files, readErr := fileutil.ReadDir(srcDir)
		if readErr != nil {
			return errfmt.Newf("read process kind directory %s", kindEntry.Name()).Wrap(readErr)
		}
		for _, file := range files {
			name := file.Name()
			if file.IsDir() || (!strings.HasSuffix(name, ".index") && name != ".index.json") {
				continue
			}
			src := filepath.Join(srcDir, name)
			dst := filepath.Join(worktreeRoot, paths.ProcessDir, kindEntry.Name(), name)
			data, readFileErr := fileutil.ReadFile(src)
			if readFileErr != nil {
				return errfmt.Newf("read runtime index %s", src).Wrap(readFileErr)
			}
			if ensureErr := fileutil.EnsureDir(filepath.Dir(dst)); ensureErr != nil {
				return errfmt.Newf("create runtime index directory %s", filepath.Dir(dst)).Wrap(ensureErr)
			}
			if writeErr := fileutil.WriteFile(dst, data, 0o600); writeErr != nil {
				return errfmt.Newf("write runtime index %s", dst).Wrap(writeErr)
			}
		}
	}
	// Isolated empty draft plane — never symlink studio object_drafts.
	// A symlink is not a directory; git status --porcelain reports dirty and
	// persistOrchestratedTaskOutcome writes status=error for every native-swarm ATK.
	// TRACK: BLI-COMMS-ORCH-DRAFT-PLANE-DIRTY-001
	worktreeDraftDir := storage.ObjectDraftPlaneRoot(worktreeRoot)
	if ensureErr := fileutil.EnsureDir(filepath.Dir(worktreeDraftDir)); ensureErr != nil {
		return errfmt.Newf("create worktree state directory").Wrap(ensureErr)
	}
	if _, statErr := fileutil.Lstat(worktreeDraftDir); statErr == nil {
		if removeErr := fileutil.RemoveAll(worktreeDraftDir); removeErr != nil {
			return errfmt.Newf("replace worktree draft plane %s", worktreeDraftDir).Wrap(removeErr)
		}
	} else if !fileutil.IsNotExist(statErr) {
		return errfmt.Newf("stat worktree draft plane %s", worktreeDraftDir).Wrap(statErr)
	}
	if ensureErr := fileutil.EnsureDir(worktreeDraftDir); ensureErr != nil {
		return errfmt.Newf("create isolated worktree draft plane").Wrap(ensureErr)
	}
	return nil
}

func persistOrchestratedTaskOutcome(
	ctx context.Context,
	state *orchestratorState,
	taskID, status string,
	output map[string]any,
) error {
	task, err := state.sp.Read(ctx, state.secCtx, taskID)
	if err != nil {
		return errfmt.Newf("read orchestrated task %s", taskID).Wrap(err)
	}
	task[objects.FieldKeyStatus] = status
	if status == objects.ObjectStatusInProgress {
		if curClaimed, _ := task[objects.FieldKeyClaimedBy].(string); strings.TrimSpace(curClaimed) == "" {
			claimant := state.opts.PersonaID
			if claimant == "" {
				if pRef, ok := task[objects.FieldKeyAssigneePersonaRef].(string); ok && pRef != "" {
					claimant = pRef
				} else if pRef, ok := task[objects.FieldKeyPersonaRef].(string); ok && pRef != "" {
					claimant = pRef
				} else {
					claimant = authcred.DefaultSwarmWorkerAccount
				}
			}
			task[objects.FieldKeyClaimedBy] = claimant
			task[objects.FieldKeyClaimedAt] = time.Now().UTC().Format(time.RFC3339)
		}
	}
	if output != nil {
		outputs, _ := task[objects.FieldKeyOutputs].([]any)
		task[objects.FieldKeyOutputs] = append(outputs, output)
		if commitSHA, _ := output["commit_sha"].(string); commitSHA != "" {
			task[objects.FieldKeyCommitHashes] = appendStringReference(
				task[objects.FieldKeyCommitHashes],
				commitSHA,
			)
		}
	}
	if err := state.sp.Update(ctx, state.secCtx, taskID, task); err != nil {
		return errfmt.Newf("persist orchestrated task %s as %s", taskID, status).Wrap(err)
	}
	return nil
}

func configureOrchestrationExecutorProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second
}

func buildOrchestrationExecutorArgs(taskID, promptMarkdown string, timeout time.Duration) []string {
	args := []string{
		"agent",
		"execute",
		"--task-id",
		taskID,
		"--prompt",
		promptMarkdown,
	}
	if timeout > 0 {
		args = append(args, "--timeout", timeout.String())
	}
	return args
}

// orchestrationExecutorChildEnv injects the seat API key and binds the child CLI to the
// seated kernel. Git cwd may be the ATK worktree; ZQK_PROJECT_ROOT must not be.
// seedAgentWorktreeRuntime copies .account.index without account YAML, so a worktree
// bind yields POL-AGENT-ACCOUNT-LOGIN-001 / CRI-ACCOUNT-RBAC-READY on DefaultSwarmWorkerAccount.
// TRACK: BLI-KERNEL-ROOT-BINDING-FAILCLOSED-001
func orchestrationExecutorChildEnv(parent []string, seatedKernelRoot, seatKey, zqkBin string) []string {
	childEnv := withLocalLLMEnv(authcred.WithSeatAPIKeyEnv(parent, seatKey, seatedKernelRoot))
	return withEnvValue(childEnv, zqkenv.Bin().Name(), zqkBin)
}

func appendStringReference(raw any, value string) []any {
	refs := make([]any, 0)
	switch existing := raw.(type) {
	case []any:
		refs = append(refs, existing...)
	case []string:
		for _, ref := range existing {
			refs = append(refs, ref)
		}
	}
	for _, ref := range refs {
		if ref == value {
			return refs
		}
	}
	return append(refs, value)
}

func collectOrchestrationCommitManifest(
	ctx context.Context,
	worktreePath, taskID, baseSHA string,
) (map[string]any, error) {
	resultSHA, err := gitWorktreeOutput(ctx, worktreePath, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	dirty, err := gitWorktreeOutput(ctx, worktreePath, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	// seedAgentWorktreeRuntime may leave .zqk/object_drafts as a symlink; ignore it.
	// TRACK: BLI-COMMS-ORCH-DRAFT-PLANE-DIRTY-001
	if remaining := orchestrationExecutorDirt(dirty); remaining != "" {
		return nil, errfmt.Errorf("executor left uncommitted work for %s: %s", taskID, remaining)
	}
	if resultSHA == baseSHA {
		return nil, errfmt.Errorf("executor produced no commit for %s", taskID)
	}
	changedOutput, err := gitWorktreeOutput(ctx, worktreePath, "diff", "--name-only", baseSHA+".."+resultSHA)
	if err != nil {
		return nil, err
	}
	changedPaths := splitNonEmptyLines(changedOutput)
	if len(changedPaths) == 0 {
		return nil, errfmt.Errorf("executor commit for %s changed no paths", taskID)
	}
	return map[string]any{
		objects.FieldKeyType:         "git_commit_manifest",
		"task_id":                    taskID,
		objects.FieldKeyBaseSha:      baseSHA,
		"result_sha":                 resultSHA,
		"commit_sha":                 resultSHA,
		objects.FieldKeyChangedPaths: changedPaths,
		"worktree":                   worktreePath,
	}, nil
}

// autoCommitWorktreeChanges commits executor changes if uncommitted work was produced and passes build check.
func autoCommitWorktreeChanges(ctx context.Context, worktreePath, taskID string, itemID ...string) error {
	dirty, err := gitWorktreeOutput(ctx, worktreePath, "status", "--porcelain")
	if err != nil {
		return err
	}
	if remaining := orchestrationExecutorDirt(dirty); remaining == "" {
		return nil
	}
	if bErr := worktreeBuildCheck(ctx, worktreePath); bErr != nil {
		return bErr
	}
	addCmd := execwrap.CommandContext(ctx, "git", "add", "-A")
	addCmd.Dir = worktreePath
	if out, err := addCmd.CombinedOutput(); err != nil {
		return errfmt.Newf("git add in %s: %s", worktreePath, strings.TrimSpace(string(out))).Wrap(err)
	}
	// Unstage config/zqk-local.yaml so worktree runtime settings are not committed
	resetCmd := execwrap.CommandContext(ctx, "git", "reset", "--", "config/zqk-local.yaml")
	resetCmd.Dir = worktreePath
	_ = resetCmd.Run()
	commitMsg := fmt.Sprintf("Agent implementation for %s", taskID)
	if len(itemID) > 0 && strings.TrimSpace(itemID[0]) != "" {
		commitMsg = fmt.Sprintf("Agent implementation for %s (%s)", taskID, strings.TrimSpace(itemID[0]))
	}
	commitCmd := execwrap.CommandContext(ctx, "git", "-c", "user.name=ZQK Swarm Agent", "-c", "user.email=swarm@zqk.internal", "commit", "-m", commitMsg)
	commitCmd.Dir = worktreePath
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return errfmt.Newf("git commit in %s: %s", worktreePath, strings.TrimSpace(string(out))).Wrap(err)
	}
	return nil
}

func gitWorktreeOutput(ctx context.Context, worktreePath string, args ...string) (string, error) {
	cmd := execwrap.CommandContext(ctx, "git", args...)
	cmd.Dir = worktreePath
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		return "", errfmt.Newf("git %s in %s: %s", strings.Join(args, " "), worktreePath, trimmed).Wrap(err)
	}
	return trimmed, nil
}

// orchestrationExecutorDirt drops seeded draft-plane porcelain so an isolated
// .zqk/object_drafts directory (or a leftover symlink) is not treated as executor leftover work.
// TRACK: BLI-COMMS-ORCH-DRAFT-PLANE-DIRTY-001
func orchestrationExecutorDirt(porcelain string) string {
	var kept []string
	for _, line := range splitNonEmptyLines(porcelain) {
		if isSeededDraftPlanePorcelain(line) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func isSeededDraftPlanePorcelain(line string) bool {
	path := line
	if len(line) >= 3 && (line[0] == ' ' || line[0] == '?' || line[0] == 'A' || line[0] == 'M' || line[0] == 'D' || line[0] == 'R' || line[0] == 'C' || line[0] == 'U' || line[0] == '!') {
		path = strings.TrimSpace(line[2:])
	}
	if _, target, ok := strings.Cut(path, " -> "); ok {
		path = strings.TrimSpace(target)
	}
	path = strings.Trim(path, `"`)
	path = strings.TrimPrefix(path, "./")
	path = filepath.ToSlash(path)
	if strings.HasSuffix(path, ".index") || strings.HasSuffix(path, ".index.json") {
		return true
	}
	if path == "config/zqk-local.yaml" || path == "config/zqk-local.yml" || strings.HasPrefix(path, "config/zqk-local.yaml") || strings.HasPrefix(path, "config/zqk-local.yml") {
		return true
	}
	return path == ".zqk/object_drafts" || strings.HasPrefix(path, ".zqk/object_drafts/")
}

func splitNonEmptyLines(value string) []string {
	var result []string
	for line := range strings.SplitSeq(value, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
