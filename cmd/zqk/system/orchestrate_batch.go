package system

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NewOrchestrateBatchCmd creates a new command to replace the bash-based orchestrate_batch logic natively
func NewOrchestrateBatchCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemOrchestrateBatchCommandBuilder(), &cobra.Command{
		Use:   "orchestrate-batch [priority_plan_id] [item_ids...]",
		Short: "Generates the ontological cascade (Workstream, Requirement, Criteria, TestCase) for a batch of items",
		Long:  `Replaces bash script orchestration with generic, native CLI logic. Builds the required cascade per the system object granularity framework.`,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
				ctx := cmd.Context()
				logger := logging.GetLoggerFromContext(ctx)
				priID := args[0]
				itemIDs := args[1:]
				sp := proc.Storage()

				for _, itemID := range itemIDs {
					logging.FluentEvent(logger).Info(fmt.Sprintf("Orchestrating %s...", itemID)).Log()

					// 1. Get original item title
					item, err := sp.Read(proc.OperationContext(), proc.SecurityContext(), itemID)
					if err != nil {
						logging.FluentEvent(logger).Warn(fmt.Sprintf("Warning: failed to read %s: %v", itemID, err)).Log()
						continue
					}

					title := "Implementation for " + itemID
					if t, ok := item[objects.FieldKeyTitle].(string); ok && t != "" {
						title = t
					}

					// 2. Create Criteria
					critData := map[string]any{
						objects.FieldKeyKind:        objects.KindCriteria,
						objects.FieldKeyTitle:       "Criteria for " + title,
						objects.FieldKeyDescription: "Acceptance criteria for " + title,
						objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
					}
					err = sp.Create(proc.OperationContext(), proc.SecurityContext(), critData)
					if err != nil {
						return err
					}
					critID := critData[objects.FieldKeyID].(string)
					logging.FluentEvent(logger).Debug(fmt.Sprintf("Created Criteria: %s", critID)).Log()

					// 3. Create Workstream
					wsData := map[string]any{
						objects.FieldKeyKind:            objects.KindWorkstream,
						objects.FieldKeyTitle:           "WS: " + title,
						objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
						objects.FieldKeyPriorityPlanRef: priID,
					}
					err = sp.Create(proc.OperationContext(), proc.SecurityContext(), wsData)
					if err != nil {
						return err
					}
					wsID := wsData[objects.FieldKeyID].(string)
					logging.FluentEvent(logger).Debug(fmt.Sprintf("Created Workstream: %s", wsID)).Log()

					// 4. Create Requirement
					reqData := map[string]any{
						objects.FieldKeyKind:         objects.KindRequirement,
						objects.FieldKeyTitle:        "REQ: " + title,
						objects.FieldKeyStatus:       objects.ObjectStatusProposed,
						objects.FieldKeyCriteriaRefs: []string{critID},
					}
					err = sp.Create(proc.OperationContext(), proc.SecurityContext(), reqData)
					if err != nil {
						return err
					}
					reqID := reqData[objects.FieldKeyID].(string)
					logging.FluentEvent(logger).Debug(fmt.Sprintf("Created Requirement: %s", reqID)).Log()

					// 5. Create Test Case
					tcData := map[string]any{
						objects.FieldKeyKind:            objects.KindTestCase,
						objects.FieldKeyTitle:           "TC: " + title,
						objects.FieldKeyStatus:          objects.ObjectStatusDraft,
						objects.FieldKeyRequirementRefs: []string{reqID},
						objects.FieldKeyWorkstreamRefs:  []string{wsID},
						objects.FieldKeyCriteriaRefs:    []string{critID},
						objects.FieldKeyOwnerRef:        objects.DefaultSystemAccountID,
					}
					err = sp.Create(proc.OperationContext(), proc.SecurityContext(), tcData)
					if err != nil {
						return err
					}
					tcID := tcData[objects.FieldKeyID].(string)
					logging.FluentEvent(logger).Debug(fmt.Sprintf("Created Test Case: %s", tcID)).Log()

					// 6. Update original item
					refsToAdd := map[string]string{
						objects.FieldKeyWorkstreamRefs:  wsID,
						objects.FieldKeyRequirementRefs: reqID,
					}

					for refKey, refVal := range refsToAdd {
						existingRefs, _ := item[refKey].([]any)
						item[refKey] = append(existingRefs, refVal)
					}

					item[objects.FieldKeyPriorityPlanRef] = priID
					item[objects.FieldKeyStatus] = "planned"

					if err := sp.Update(proc.OperationContext(), proc.SecurityContext(), itemID, item); err != nil {
						return err
					}
					logging.FluentEvent(logger).Info(fmt.Sprintf("Updated %s", itemID)).Log()
				}

				return nil
			})(cmd, args)
		},
	})
	return cmd
}
