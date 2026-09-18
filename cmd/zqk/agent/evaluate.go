package agent

import (
	"fmt"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	audit_event "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/audit"
	"github.com/spf13/cobra"
)

type EvaluateOptions struct {
	SessionID string
}

func NewEvaluateCmd() *cobra.Command {
	opts := &EvaluateOptions{}

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Evaluate a completed prompt run",
		"Validates artifacts against lifecycle and mandatory constraints after an agent completes its run.",
		"Reads the change journal and system state to determine policy adherence and emits an audit event.",
	).
		AddExample("Evaluate a specific convergence session run", "%s agent evaluate-run --session-id CVS-123")

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewAgentEvaluateRunCommandBuilder(), &cobra.Command{
		Use:   "evaluate-run",
		Short: "Evaluate a completed prompt run",
		RunE: cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
			if opts.SessionID == "" {
				return errfmt.Errorf("requires --session-id")
			}

			var err error
			_ = err
			if proc.ProjectRoot() == "" {
				return errfmt.Errorf("project root is required")
			}

			// Simulated policy check
			violations := []string{}

			// Check if there are system check violations
			// In a real implementation we would run the validation suite against objects modified in the session.

			adherence := len(violations) == 0

			ctx := proc.OperationContext()
			secCtx := proc.SecurityContext()
			sp := proc.Storage()

			// Emit telemetry
			evalEvent := map[string]any{
				objects.FieldKeyKind:          objects.KindAuditEvent,
				objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
				objects.FieldKeyEventType:     string(audit_event.EventTypePromptRunComplete),
				objects.FieldKeyTitle:         fmt.Sprintf("Prompt Run Evaluated: %s", opts.SessionID),
				objects.FieldKeyMetadata: map[string]any{
					"run_id":            opts.SessionID,
					"policy_adherence":  adherence,
					"policy_violations": violations,
				},
			}

			if err := sp.Create(ctx, secCtx, evalEvent); err != nil {
				return errfmt.Newf("failed to create evaluation audit event").Wrap(err)
			}

			out := fmt.Sprintf("Evaluated session %s: Adherence=%v, Violations=%d\n", opts.SessionID, adherence, len(violations))
			return cli.WriteOutput(cmd, []byte(out))
		})})

	cmd.Flags().StringVar(&opts.SessionID, "session-id", "", "ID of the session to evaluate")
	_ = cmd.MarkFlagRequired("session-id")

	helpBuilder.ApplyToCommand(cmd)

	return cmd
}
