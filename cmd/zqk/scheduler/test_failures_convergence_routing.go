package scheduler

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

// resolveConvergenceRoutingForCLI loads convergence_session fields when --session-id is set (unless
// --skip-session-context). Flags override object fields. Emits metadata for suggested_convergence_session_fields.
// When the session object is read successfully, beforeStateSnapshot is the persisted before_state_snapshot
// (iteration tombstone) for disparity vs current measurement. predictions is the session's predictions
// map (for debrief vs health snapshot), or nil if unread.
func resolveConvergenceRoutingForCLI(cmd *cobra.Command, sessionID, flagCurrentPhase, flagFlowVariant string, skipSessionContext bool) (effCurrentPhase, effFlowVariant string, meta map[string]any, beforeStateSnapshot map[string]any, predictions map[string]any, sessionThresholds map[string]any, err error) {
	if strings.TrimSpace(sessionID) == emptyValue {
		return flagCurrentPhase, flagFlowVariant, nil, nil, nil, nil, nil
	}
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return "", "", nil, nil, nil, nil, err
	}
	return schedpkg.ResolveConvergenceRoutingSession(proc.OperationContext(), proc.Storage(), proc.SecurityContext(), sessionID, flagCurrentPhase, flagFlowVariant, skipSessionContext)
}
