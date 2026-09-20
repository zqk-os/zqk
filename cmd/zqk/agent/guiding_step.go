package agent

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/interactionpolicy"
	"github.com/zqk-os/zqk/pkg/logging"
)

// NewGuidingStepCmd returns zqk agent guiding-step (kernel ping-pong).
func NewGuidingStepCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentGuidingStepCommandBuilder()
	cmd.RunE = runAgentGuidingStep
	return cmd
}

func runAgentGuidingStep(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		event, _ := cmd.Flags().GetString("event")
		shell, _ := cmd.Flags().GetString("shell")
		personaRef, _ := cmd.Flags().GetString("persona-ref")
		event = strings.TrimSpace(event)
		shell = strings.TrimSpace(shell)
		personaRef = strings.TrimSpace(personaRef)

		resolved, unmatchedShell := resolveGuidingEvent(event, shell)
		if unmatchedShell {
			// afterShellExecution pings every command. Unclassified --shell is
			// not a failure (legal $TMPDIR/zqk-worktrees adds, object get, …).
			return cli.FormatOutput(cmd, unmatchedGuidingStepPayload())
		}
		if resolved == "" {
			return errfmt.Errorf("requires --event or --shell")
		}
		event = resolved

		ctx := proc.OperationContext()
		sec := proc.SecurityContext()
		if sec == nil {
			sec = pkgctx.NewSystemSecurityContext()
		}
		sp := proc.Storage()

		var persona map[string]any
		if personaRef != "" {
			if sp == nil {
				return errfmt.Errorf("storage unavailable")
			}
			obj, err := sp.Read(ctx, sec, personaRef)
			if err != nil {
				return errfmt.Newf("read persona %s", personaRef).Wrap(err)
			}
			persona = obj
		}

		res := interactionpolicy.Evaluate(persona, event)
		hydrated := false
		if res.Step != nil && sp != nil && res.Step.PolicyID != "" {
			pol, err := sp.Read(ctx, sec, res.Step.PolicyID)
			if err == nil && interactionpolicy.OverlayFromPolicy(res.Step, pol) {
				hydrated = true
			}
		}

		if res.Matched && res.Step != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileHuman))
			if proc.Context() != nil && proc.Context().Profile != "" {
				logger = logging.GetLoggerFromProfile(proc.Context().Profile)
			}
			logging.Fluent(logger).Info("guiding_step").
				String("event", res.Event).
				String("policy_id", res.Step.PolicyID).
				Bool("hydrated", hydrated).
				Log()
		}

		payload := map[string]any{
			"matched":  res.Matched,
			"event":    res.Event,
			"hydrated": hydrated,
		}
		if res.Step != nil {
			payload["policy_id"] = res.Step.PolicyID
			payload["guiding_step"] = res.Step.GuidingStep
			if res.Step.CommandHint != "" {
				payload["command_hint"] = res.Step.CommandHint
			}
		}
		return cli.FormatOutput(cmd, payload)
	})(cmd, nil)
}

// resolveGuidingEvent maps --event / --shell to a catalog class.
// unmatchedShell is true when --shell was set but ClassifyShell produced nothing.
func resolveGuidingEvent(event, shell string) (resolved string, unmatchedShell bool) {
	event = strings.TrimSpace(event)
	shell = strings.TrimSpace(shell)
	if event != "" {
		return event, false
	}
	if shell == "" {
		return "", false
	}
	if got := interactionpolicy.ClassifyShell(shell); got != "" {
		return got, false
	}
	return "", true
}

func unmatchedGuidingStepPayload() map[string]any {
	return map[string]any{
		"matched":  false,
		"event":    "",
		"hydrated": false,
	}
}
