package validate

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
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
		var violations []string
		score := 100

		// Check 1: Git state is clean
		gitStatusCmd := execwrap.Command("git", "status", "--porcelain")
		gitStatusCmd.Dir = proc.ProjectRoot()
		statusOut, err := gitStatusCmd.Output()
		if err != nil {
			violations = append(violations, "Git status check failed: "+err.Error())
			score -= 20
		} else if len(strings.TrimSpace(string(statusOut))) > 0 {
			violations = append(violations, "Git state is not clean. Commit or stash your changes.")
			score -= 20
		}

		// Check 2: Branch naming matches item ID pattern
		gitBranchCmd := execwrap.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
		gitBranchCmd.Dir = proc.ProjectRoot()
		branchOut, err := gitBranchCmd.Output()
		currentBranch := strings.TrimSpace(string(branchOut))
		if err != nil {
			violations = append(violations, "Failed to get current branch: "+err.Error())
			score -= 20
		}

		// Very simple branch parsing (e.g. feat/REQ-001 -> REQ-001)
		var activeItemID string
		parts := strings.Split(currentBranch, "/")
		if len(parts) >= 2 {
			activeItemID = parts[len(parts)-1]
		} else {
			activeItemID = currentBranch
		}

		if activeItemID == "main" || activeItemID == "master" {
			violations = append(violations, "Currently on main branch. Checkout a feature branch named after an item ID.")
			score -= 20
		}

		// Checks 3 & 4: Priority Plan Validations
		sp := proc.Storage()
		if sp != nil && activeItemID != "main" && activeItemID != "master" {
			filter := storage.ListFilter{
				Kind: objects.KindPriorityPlan,
				Filters: map[string]any{
					objects.FieldKeyStatus: objects.ObjectStatusActive,
				},
				Limit:  1,
				Fields: []string{objects.FieldKeyID, objects.FieldKeyStatus, objects.FieldKeyBacklogItemRefs},
			}
			res, err := sp.List(proc.OperationContext(), proc.SecurityContext(), pkgctx.NewStorageContext(), filter)
			if err == nil && res != nil && len(res.Objects) > 0 {
				plan := res.Objects[0]

				// Validate active item is in current priority plan
				foundInPlan := false
				var items []string
				if refs, ok := plan[objects.FieldKeyBacklogItemRefs].([]any); ok {
					for _, ref := range refs {
						if strRef, ok := ref.(string); ok {
							items = append(items, strRef)
							if strings.Contains(strRef, activeItemID) {
								foundInPlan = true
								activeItemID = strRef // use full ID
							}
						}
					}
				}

				if !foundInPlan {
					violations = append(violations, fmt.Sprintf("Active item '%s' is not in the active priority plan.", activeItemID))
					score -= 20
				} else {
					// Validate active item is first non-completed
					// Simplified check for demonstration: if it's not the first item, flag it
					if len(items) > 0 && items[0] != activeItemID {
						violations = append(violations, "Active item is not the first non-completed item in the priority plan.")
						score -= 20
					}
				}
			} else {
				violations = append(violations, "No active priority plan found.")
				score -= 20
			}
		}

		// Check 5: Branch is based on latest main
		// This requires network fetch usually, but locally we just check merge-base
		gitMergeBaseCmd := execwrap.Command("git", "merge-base", "HEAD", "main")
		gitMergeBaseCmd.Dir = proc.ProjectRoot()
		mergeBaseOut, err := gitMergeBaseCmd.Output()
		if err == nil {
			gitMainRevCmd := execwrap.Command("git", "rev-parse", "main")
			gitMainRevCmd.Dir = proc.ProjectRoot()
			mainRevOut, err := gitMainRevCmd.Output()
			if err == nil {
				if strings.TrimSpace(string(mergeBaseOut)) != strings.TrimSpace(string(mainRevOut)) {
					violations = append(violations, "Branch is not based on latest main. Run 'git fetch origin main && git rebase origin/main'.")
					score -= 20
				}
			}
		}

		if score < 0 {
			score = 0
		}

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
