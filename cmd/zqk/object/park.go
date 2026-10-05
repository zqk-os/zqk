package object

import (
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

// Park destinations are intentional lateral / sequencing exits — not promote (forward)
// and not demote (retreat). TRACK: park verb until full halt/sequencing CLI DNA.
var parkAllowedTargets = map[string]struct{}{
	objects.ObjectStatusDeferred: {},
	objects.ObjectStatusRoadmap:  {},
	objects.ObjectStatusArchived: {},
}

// NewParkCmd creates object park — move to deferred, roadmap, or archived along a real lifecycle edge.
func NewParkCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectParkCommandBuilder()
	cli.BindAsyncProgress(cmd, runPark)
	return cmd
}

func runPark(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		to, _ := cmd.Flags().GetString("to")
		to = strings.TrimSpace(strings.ToLower(to))
		if to == emptyValue {
			return errfmt.Errorf("object park requires --to (deferred|roadmap|archived)")
		}
		if _, ok := parkAllowedTargets[to]; !ok {
			return errfmt.Errorf("object park --to must be deferred, roadmap, or archived (got %q)", to)
		}
		args = expandObjectIDArgs(cmd, args)
		if len(args) == 0 {
			return errfmt.Errorf("object park requires at least one object id (positional, comma-separated, and/or --ids)")
		}

		ctx := proc.OperationContext()
		secCtx := proc.SecurityContext()
		lifecyclesDir := filepath.Join(proc.ProjectRoot(), paths.ProcessInternalLifecyclesDir)
		lifecycleLoader := objects.NewLifecycleLoader(lifecyclesDir)

		seeds := make([]string, 0, len(args))
		for _, idArg := range args {
			id, err := proc.ResolveSemanticArgument(ctx, "", idArg)
			if err != nil {
				return errfmt.Errorf("object park: failed to resolve ID %s: %w", idArg, err)
			}
			seeds = append(seeds, id)
		}

		dependents := func(id string) []string {
			return storage.DependentsForID(ctx, proc.Storage(), id)
		}
		plan, err := lifecycle.PlanStageMembraneHop(ctx, secCtx, proc.Storage(), lifecycleLoader, dependents, seeds, to)
		if err != nil {
			return err
		}

		if err := lifecycle.ApplyStageMembraneHop(ctx, secCtx, proc.Storage(), dependents, plan); err != nil {
			return err
		}

		flushTracker := newFlushKindTracker(len(plan.Members))
		parked := make([]map[string]any, 0, len(plan.Members))
		skipped := make([]string, 0)
		for _, m := range plan.Members {
			flushTracker.addWithBacklogCascade(m.Kind)
			if m.Skip {
				skipped = append(skipped, m.ID)
				logging.FluentEvent(proc.Logger()).Info("Already parked at target").
					ObjectID(m.ID).String("status", to).Log()
				continue
			}
			logging.FluentEvent(proc.Logger()).Info("Parked object").
				ObjectID(m.ID).String("from", m.From).String("to", to).Kind(m.Kind).Log()
			parked = append(parked, map[string]any{
				objects.FieldKeyID:     m.ID,
				objects.FieldKeyKind:   m.Kind,
				"from_status":          m.From,
				objects.FieldKeyStatus: to,
			})
		}

		flushObjectMutationVisibility(proc, "park", flushTracker.kinds())

		return cli.FormatOutput(cmd, map[string]any{
			"parked":                parked,
			objects.FieldKeySkipped: skipped,
			"to":                    to,
			"cluster":               plan.ClusterIDs(),
			"seeds":                 seeds,
			"shockwave_mode":        plan.Policy.Mode,
			"lineage_parents":       plan.LineageParents,
		})
	})(cmd, args)
}

func lifecycleAllowsTransition(lc *objects.Lifecycle, from, to string) bool {
	return lifecycle.TransitionAllowed(lc, from, to)
}
