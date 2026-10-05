package agent

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	audit_event "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/audit"
)

func NewEvaluateCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentEvaluateRunCommandBuilder()
	cmd.RunE = cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		sessionID, _ := cmd.Flags().GetString("session-id")
		if sessionID == "" {
			return errfmt.Errorf("requires --session-id")
		}

		if proc.ProjectRoot() == "" {
			return errfmt.Errorf("project root is required")
		}

		// Simulated policy check
		violations := []string{}

		adherence := len(violations) == 0

		ctx, secCtx, sp := proc.StorageTuple()

		// Emit telemetry
		evalEvent := map[string]any{
			objects.FieldKeyKind:          objects.KindAuditEvent,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyEventType:     string(audit_event.EventTypePromptRunComplete),
			objects.FieldKeyTitle:         fmt.Sprintf("Prompt Run Evaluated: %s", sessionID),
			objects.FieldKeyMetadata: map[string]any{
				"run_id":            sessionID,
				"policy_adherence":  adherence,
				"policy_violations": violations,
			},
		}

		if err := sp.Create(ctx, secCtx, evalEvent); err != nil {
			return errfmt.Newf("failed to create evaluation audit event").Wrap(err)
		}

		out := fmt.Sprintf("Evaluated session %s: Adherence=%v, Violations=%d\n", sessionID, adherence, len(violations))
		return cli.WriteOutput(cmd, []byte(out))
	})
	return cmd
}
