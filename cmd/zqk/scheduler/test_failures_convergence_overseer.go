package scheduler

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/convergerollup"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

func runTestFailuresConvergenceOverseerFromCmd(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	return runTestFailuresConvergenceOverseer(ctx, cmd)
}

func runTestFailuresConvergenceOverseer(cliCtx *cli.Context, cmd *cobra.Command) error {
	projectRoot := cliCtx.ProjectRoot
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
		if projectRoot == emptyValue {
			return errfmt.Errorf("project root not found")
		}
	}
	coord, _ := cmd.Flags().GetString("coordinator-session-id")
	coord = strings.TrimSpace(coord)
	if coord == emptyValue {
		return errfmt.Errorf("--coordinator-session-id is required")
	}
	limit, _ := cmd.Flags().GetInt("limit")
	if limit <= 0 {
		limit = 500
	}
	skipRollupGates, _ := cmd.Flags().GetBool("skip-rollup-gates")

	lines, err := readTestBundleHealthTailForCLI(cmd, projectRoot, limit)
	if err != nil {
		if errors.Is(err, errTestBundleHealthFileHandled) {
			lines = nil
		} else {
			return err
		}
	}
	snap := schedpkg.BuildTestBundleConvergenceSnapshot(projectRoot, lines)
	rollupCore, err := buildRollupStatusCore(cmd, projectRoot, false, coord, snap, skipRollupGates)
	if err != nil {
		return err
	}
	tree, err := collectCVSTreeForOverseer(cmd, coord, convergerollup.DefaultOverseerCVSTreeMaxDepth)
	if err != nil {
		return errfmt.Newf("cvs tree").Wrap(err)
	}

	var cyclePath []string
	proc, perr := cli.NewProcessor(cmd)
	if perr == nil {
		opCtx := proc.OperationContext()
		sec := proc.SecurityContext()
		refsFor := func(id string) ([]string, error) {
			obj, rerr := proc.Storage().Read(opCtx, sec, id)
			if rerr != nil {
				return nil, rerr
			}
			return cvsIDsFromRelatedObjectRefs(obj[objects.FieldKeyRelatedObjectRefs]), nil
		}
		if c, ok := convergerollup.DetectCVSRefCycle(coord, refsFor, convergerollup.MaxRelatedCVSHopDepth); ok {
			cyclePath = c
		}
	}

	arbitrated := convergerollup.BuildArbitratedParentMessage(rollupCore, tree, coord)
	out := map[string]any{
		"coordinator_session_id":          coord,
		"cvs_tree":                        tree,
		"rollup_status_core":              rollupCore,
		"arbitrated_next_action_markdown": arbitrated,
		"related_object_refs_cycle":       nil,
		"max_tree_depth":                  convergerollup.DefaultOverseerCVSTreeMaxDepth,
		"evaluation_note":                 "Full vetting matrix / drift baselines: scripts/cvs_outcome_rollup.py. Tree is BFS deduplicated by CVS id.",
	}
	if len(cyclePath) > 0 {
		out["related_object_refs_cycle"] = cyclePath
	}

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, out)
	default:
		var sb strings.Builder
		sb.WriteString("Convergence overseer (coordinator-scoped)\n\n")
		fmt.Fprintf(&sb, "Coordinator: %s\n", coord)
		if rs, ok := rollupCore["rollup_status"].(string); ok {
			fmt.Fprintf(&sb, "rollup_status: %s\n", rs)
		}
		if r, ok := rollupCore["ready_for_parent_completion"].(bool); ok {
			fmt.Fprintf(&sb, "ready_for_parent_completion: %v\n", r)
		}
		fmt.Fprintf(&sb, "CVS tree nodes: %d (max depth %d)\n", len(tree), convergerollup.DefaultOverseerCVSTreeMaxDepth)
		if len(cyclePath) > 0 {
			fmt.Fprintf(&sb, "related_object_refs_cycle: %s\n", strings.Join(cyclePath, " -> "))
		}
		sb.WriteString("\n")
		sb.WriteString(arbitrated)
		sb.WriteString("\n")
		return cli.WriteOutput(cmd, []byte(sb.String()))
	}
}
