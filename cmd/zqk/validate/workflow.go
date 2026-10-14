package validate

import (
	"context"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewCheckCmd returns the workflow check command.
func NewCheckCmd() *cobra.Command {
	builder := clipkg.NewCommandBuilder("workflow")
	builder.WithShort("Validates adherence to the prescriptive workflow")
	help := clipkg.DynamicHelpBuilder("Validates adherence to the prescriptive workflow")
	help.WithDescriptionLines("Validates branch naming, active item in current plan, git state, and branch base.")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(0))
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})

	cmd := builder.Build()
	cli.BindAsyncProgress(cmd, runCheck)
	cli.RequireStorage(cmd, true)
	return cmd
}

func runCheck(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		violations, score := evaluateWorkflowAdherence(proc)

		out := fmt.Sprintf("Workflow Adherence Score: %d%%\n\n", score)
		if len(violations) > 0 {
			out += "Violations:\n"
			for i, v := range violations {
				out += fmt.Sprintf("  %d. %s\n", i+1, v)
			}
		} else {
			out += "Perfect adherence! No violations found.\n"
		}

		return cli.WriteOutput(cmd, []byte(out))
	})(cmd, args)
}

func evaluateWorkflowAdherence(proc *cli.Processor) ([]string, int) {
	var violations []string
	score := 100

	if v, ok := checkGitClean(proc.ProjectRoot()); !ok {
		violations = append(violations, v)
		score -= 20
	}

	activeItemID, v, ok := checkBranchNaming(proc.ProjectRoot())
	if !ok {
		violations = append(violations, v)
		score -= 20
	}

	sp := proc.Storage()
	if sp != nil && activeItemID != "main" && activeItemID != "master" {
		if v, ok := checkPriorityPlanAdherence(proc.OperationContext(), proc.SecurityContext(), sp, activeItemID); !ok {
			violations = append(violations, v)
			score -= 20
		}
	}

	if v, ok := checkBranchBase(proc.ProjectRoot()); !ok {
		violations = append(violations, v)
		score -= 20
	}

	if score < 0 {
		score = 0
	}
	return violations, score
}

func checkGitClean(projectRoot string) (string, bool) {
	gitStatusCmd := execwrap.Command("git", "status", "--porcelain")
	gitStatusCmd.Dir = projectRoot
	statusOut, err := gitStatusCmd.Output()
	if err != nil {
		return "Git status check failed: " + err.Error(), false
	}
	if len(strings.TrimSpace(string(statusOut))) > 0 {
		return "Git state is not clean. Commit or stash your changes.", false
	}
	return "", true
}

func checkBranchNaming(projectRoot string) (string, string, bool) {
	gitBranchCmd := execwrap.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	gitBranchCmd.Dir = projectRoot
	branchOut, err := gitBranchCmd.Output()
	currentBranch := strings.TrimSpace(string(branchOut))
	if err != nil {
		return "", "Failed to get current branch: " + err.Error(), false
	}

	var activeItemID string
	parts := strings.Split(currentBranch, "/")
	if len(parts) >= 2 {
		activeItemID = parts[len(parts)-1]
	} else {
		activeItemID = currentBranch
	}

	if activeItemID == "main" || activeItemID == "master" {
		return activeItemID, "Currently on main branch. Checkout a feature branch named after an item ID.", false
	}
	return activeItemID, "", true
}

func checkPriorityPlanAdherence(ctx context.Context, secCtx *pkgctx.SecurityContext, sp workflowStorage, activeItemID string) (string, bool) {
	filter := storage.ListFilter{
		Kind: objects.KindPriorityPlan,
		Filters: map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
		Limit:  1,
		Fields: []string{objects.FieldKeyID, objects.FieldKeyStatus, objects.FieldKeyBacklogItemRefs},
	}
	res, err := sp.List(ctx, secCtx, nil, filter)
	if err != nil || res == nil || len(res.Objects) == 0 {
		return "No active priority plan found.", false
	}

	plan := res.Objects[0]
	foundInPlan := false
	var items []string
	switch refs := plan[objects.FieldKeyBacklogItemRefs].(type) {
	case []string:
		for _, strRef := range refs {
			items = append(items, strRef)
			if strings.Contains(strRef, activeItemID) {
				foundInPlan = true
				activeItemID = strRef
			}
		}
	case []any:
		for _, ref := range refs {
			if strRef, ok := ref.(string); ok {
				items = append(items, strRef)
				if strings.Contains(strRef, activeItemID) {
					foundInPlan = true
					activeItemID = strRef
				}
			}
		}
	}

	if !foundInPlan {
		return fmt.Sprintf("Active item '%s' is not in the active priority plan.", activeItemID), false
	}
	if len(items) > 0 && items[0] != activeItemID {
		return "Active item is not the first non-completed item in the priority plan.", false
	}
	return "", true
}

func checkBranchBase(projectRoot string) (string, bool) {
	gitMergeBaseCmd := execwrap.Command("git", "merge-base", "HEAD", "main")
	gitMergeBaseCmd.Dir = projectRoot
	mergeBaseOut, err := gitMergeBaseCmd.Output()
	if err != nil {
		return "", true
	}
	gitMainRevCmd := execwrap.Command("git", "rev-parse", "main")
	gitMainRevCmd.Dir = projectRoot
	mainRevOut, err := gitMainRevCmd.Output()
	if err == nil {
		if strings.TrimSpace(string(mergeBaseOut)) != strings.TrimSpace(string(mainRevOut)) {
			return "Branch is not based on latest main. Run 'git fetch origin main && git rebase origin/main'.", false
		}
	}
	return "", true
}
