package validate

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

var stewardTickerInterval = 10 * time.Second

// NewStewardCmd creates the agent steward command
func NewStewardCmd() *cobra.Command {
	var c cobra.Command
	cmd := &c
	cmd.Use = "steward"
	cmd.Short = "Run the Roadmap Steward agent loop"
	cmd.Long = "Runs the background agent responsible for grooming, pre-flighting plans, ensuring links, and maintaining roadmap governance before execution."

	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		return runStewardLoop(cmd)
	})

	cli.AddCommonFlags(cmd)
	return cmd
}

func runStewardLoop(cmd *cobra.Command) error {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return err
	}
	ctx := proc.OperationContext()
	secCtx := proc.SecurityContext()
	sp := proc.Storage()
	log := logging.FluentEvent(logging.GetLogger())

	_ = cli.WriteOutput(cmd, []byte("🛡️ Roadmap Steward agent loop started. Monitoring 'grooming' and 'planned' objects...\n"))

	ticker := time.NewTicker(stewardTickerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// 1. Scan for grooming priority plans
			filter := storage.ListFilter{
				Kind: objects.KindPriorityPlan,
				Filters: map[string]any{
					objects.FieldKeyStatus: objects.ObjectStatusGrooming,
				},
			}
			res, err := sp.List(ctx, secCtx, nil, filter)
			if err != nil {
				log.Error("Failed to list grooming plans", err).Log()
				continue
			}

			for _, obj := range res.Objects {
				planID, _ := obj[objects.FieldKeyID].(string)

				// Steward Preflight Check
				// A priority plan must have at least one valid backlog item linked to go active.
				var hasBlis bool
				switch blis := obj[objects.FieldKeyBacklogItemRefs].(type) {
				case []any:
					hasBlis = len(blis) > 0
				case []string:
					hasBlis = len(blis) > 0
				}
				if !hasBlis {
					// Child->Parent: query for backlog_items referencing this plan via priority_plan_ref
					bliFilter := storage.ListFilter{
						Kind: objects.KindBacklogItem,
						Filters: map[string]any{
							objects.FieldKeyPriorityPlanRef: planID,
						},
						Limit: 1,
					}
					bliRes, bliErr := sp.List(ctx, secCtx, nil, bliFilter)
					if bliErr == nil && bliRes != nil && len(bliRes.Objects) > 0 {
						hasBlis = true
					}
				}
				if !hasBlis {
					_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("⏳ Steward: Plan %s is grooming but lacks backlog items. Waiting...\n", planID)))
					continue
				}

				_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("🛡️ Steward: Plan %s has backlog items. Preflight passed. Promoting to active...\n", planID)))

				// Promote to active
				promoteCtx := pkgctx.WithLifecycleBreakGlass(ctx, "steward roadmap auto-promotion")
				err = sp.Update(promoteCtx, secCtx, planID, map[string]any{
					objects.FieldKeyStatus: objects.ObjectStatusActive,
				})
				if err != nil {
					log.Error("Steward failed to promote plan", err).Log()
					_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("❌ Steward: Failed to promote plan %s: %v\n", planID, err)))
				} else {
					_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("✅ Steward: Plan %s successfully promoted to active.\n", planID)))
				}
			}
		}
	}
}
