package agent

import (
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewStatusCmd creates the agent status command
func NewStatusCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Visualize the active hive state and orchestration status",
		"Displays concurrent worker status, task routing paths, and policy compliance heartbeat.",
		"",
		"This provides real-time observability into the multi-agent orchestrator.",
	).
		AddExample("Show hive status", "%s agent status")

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewAgentStatusCommandBuilder(), &cobra.Command{
		RunE: runStatus,
	})

	helpBuilder.ApplyToCommand(cmd)
	cli.AddCommonFlags(cmd)

	return cmd
}

func runStatus(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err
		_ = proc.OperationContext() // Silence unused proc but keep context init

		// Fetch goroutine budget status as a proxy for hive activity
		budget := goroutinelabels.DefaultBudget()
		if budget == nil {
			budget = goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{MaxTotal: 100})
			goroutinelabels.SetDefaultBudget(budget)
		}

		var activeAgents []string
		var latestModTime time.Time
		storageProvider := proc.Storage()

		if storageProvider != nil {
			result, err := storageProvider.List(cmd.Context(), pkgctx.NewSystemSecurityContext(), pkgctx.NewStorageContext(), storage.ListFilter{
				Kind: objects.KindAgentTask,
				Filters: map[string]any{
					objects.FieldKeyStatus: map[string]any{"$in": []string{objects.ObjectStatusInProgress, objects.ObjectStatusPendingVerification, objects.ObjectStatusExecuting, "routed"}},
				},
			})
			if err == nil && result != nil {
				for _, obj := range result.Objects {
					title, _ := obj[objects.FieldKeyTitle].(string)
					status, _ := obj[objects.FieldKeyStatus].(string)
					id, _ := obj[objects.FieldKeyID].(string)
					activeAgents = append(activeAgents, fmt.Sprintf("%s [%s] - %s", id, status, title))

					updatedAtStr, _ := obj[objects.FieldKeyUpdatedAt].(string)
					if updatedAtStr != "" {
						if modTime, pErr := time.Parse(time.RFC3339, updatedAtStr); pErr == nil {
							if modTime.After(latestModTime) {
								latestModTime = modTime
							}
						}
					}
				}
			}
		}

		if len(activeAgents) == 0 {
			activeAgents = []string{"No active swarm workers"}
		}

		var stalledPlans []string
		if storageProvider != nil && (len(activeAgents) == 0 || activeAgents[0] == "No active swarm workers") {
			planRes, pErr := storageProvider.List(cmd.Context(), pkgctx.NewSystemSecurityContext(), pkgctx.NewStorageContext(), storage.ListFilter{
				Kind: objects.KindPriorityPlan,
				Filters: map[string]any{
					objects.FieldKeyStatus: objects.ObjectStatusInProgress,
				},
			})
			if pErr == nil && planRes != nil {
				for _, obj := range planRes.Objects {
					pTitle, _ := obj[objects.FieldKeyTitle].(string)
					pID, _ := obj[objects.FieldKeyID].(string)
					stalledPlans = append(stalledPlans, fmt.Sprintf("%s (%s) - 0 active workers (resume: %s)", pID, pTitle, paths.CLIUsage("agent", "orchestrate", pID)))
				}
			}
		}

		var budgetInfo map[string]any
		if budget != nil {
			activeCount := 0
			if len(activeAgents) > 0 && activeAgents[0] != "No active swarm workers" {
				activeCount = len(activeAgents)
			}
			maxTotal := budget.MaxTotal()
			budgetInfo = map[string]any{
				"total_capacity": maxTotal,
				"active_count":   activeCount,
				"available":      maxTotal - activeCount,
			}
		}

		policyCompliance := HeartbeatComplianceFromModTime(latestModTime)
		if len(stalledPlans) > 0 {
			policyCompliance = "stalled_swarm_alert"
		}

		hiveState := map[string]any{
			"orchestrator_status": "online",
			"active_sub_agents":   activeAgents,
			"stalled_plans":       stalledPlans,
			"worker_budget":       budgetInfo,
			"policy_compliance":   policyCompliance,
		}

		format := cli.GetFormat(cmd)
		if format == cli.FormatJSON || format == cli.FormatYAML {
			return cli.FormatOutput(cmd, hiveState)
		}

		out := "ZQK Orchestrated Hive Status\n"
		out += "============================\n\n"
		out += fmt.Sprintf("Orchestrator: %s\n", hiveState["orchestrator_status"])
		out += fmt.Sprintf("Policy Compliance: %s\n", hiveState["policy_compliance"])

		if budget != nil {
			activeCount := 0
			if len(activeAgents) > 0 && activeAgents[0] != "No active swarm workers" {
				activeCount = len(activeAgents)
			}
			maxTotal := budget.MaxTotal()
			out += "\nWorker Resource Budget:\n"
			out += fmt.Sprintf("  Active Workers: %d / %d\n", activeCount, maxTotal)
			out += fmt.Sprintf("  Available:      %d\n", maxTotal-activeCount)
		}

		out += "\nActive Sub-Agents/Tasks:\n"
		for _, agent := range activeAgents {
			out += fmt.Sprintf("  - %s\n", agent)
		}

		if len(stalledPlans) > 0 {
			out += "\n⚠️  Stalled In-Progress Plans (0 active workers):\n"
			for _, sp := range stalledPlans {
				out += fmt.Sprintf("  - %s\n", sp)
			}
		}

		return cli.WriteOutput(cmd, []byte(out))
	})(cmd, args)
}

// HeartbeatComplianceFromModTime maps the latest agent_task updated_at to policy compliance text.
func HeartbeatComplianceFromModTime(latestModTime time.Time) string {
	if latestModTime.IsZero() {
		return "no_activity"
	}
	if time.Since(latestModTime) < 15*time.Minute {
		return "heartbeat_ok"
	}
	return "heartbeat_stale"
}
