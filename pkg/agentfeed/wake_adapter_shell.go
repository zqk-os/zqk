package agentfeed

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/execwrap"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// ShellPeerWakeAdapter is the legacy local membrane that invokes project scripts.
// Kept as one replaceable adapter — do not scatter shell basenames through core feed logic.
//
// TRACK: CRIT-COMMS-003 — retire when MCP/HTTP wake adapters are the default product path.
type ShellPeerWakeAdapter struct {
	// WorkerScript / CoordinatorScript override basenames under paths.ScriptsDirPath.
	WorkerScript      string
	CoordinatorScript string
}

const (
	shellWakeWorkerScript      = "wake-agy.sh"
	shellWakeCoordinatorScript = "wake-peer-tpm-02.sh"
)

// NewShellPeerWakeAdapter returns the default studio shell membrane adapter.
func NewShellPeerWakeAdapter() *ShellPeerWakeAdapter {
	return &ShellPeerWakeAdapter{
		WorkerScript:      shellWakeWorkerScript,
		CoordinatorScript: shellWakeCoordinatorScript,
	}
}

// Wake implements PeerWakeAdapter.
func (a *ShellPeerWakeAdapter) Wake(ctx context.Context, req PeerWakeRequest) (PeerWakeAdapterResult, error) {
	if a == nil {
		a = NewShellPeerWakeAdapter()
	}
	script := a.scriptPath(req.ProjectRoot, req.SeatKind)
	if !fileutil.IsRegularFile(script) {
		return PeerWakeAdapterResult{Endpoint: script}, errfmt.Errorf("wake membrane missing: %s", script)
	}
	args := a.args(req)
	cmd := execwrap.CommandContext(ctx, script, args...)
	cmd.Dir = req.ProjectRoot
	out, err := cmd.CombinedOutput()
	res := PeerWakeAdapterResult{
		Endpoint:  script,
		Transport: a.transport(req),
		Live:      a.live(req),
		Output:    strings.TrimSpace(string(out)),
	}
	if err != nil {
		return res, errfmt.Errorf("%w: %s", err, res.Output)
	}
	return res, nil
}

func (a *ShellPeerWakeAdapter) scriptPath(projectRoot, seatKind string) string {
	name := a.WorkerScript
	if name == "" {
		name = shellWakeWorkerScript
	}
	if seatKind == SeatKindCoordinator {
		name = a.CoordinatorScript
		if name == "" {
			name = shellWakeCoordinatorScript
		}
	}
	return filepath.Join(paths.ScriptsDirPath(projectRoot), name)
}

func (a *ShellPeerWakeAdapter) args(req PeerWakeRequest) []string {
	if req.SeatKind == SeatKindCoordinator {
		return wakeScriptArgsForTPM(req.DeliveryMode, req.PasteText)
	}
	return wakeScriptArgsForSeat(req.DeliveryMode, req.PasteText, req.PeerPID, req.Conversation)
}

func (a *ShellPeerWakeAdapter) transport(req PeerWakeRequest) string {
	if req.SeatKind == SeatKindCoordinator {
		if strings.TrimSpace(req.DeliveryMode) == datacell.DeliveryModePaste {
			return TransportTPMPaste
		}
		return TransportTPMStamp
	}
	return TransportAgentAPINotify
}

func (a *ShellPeerWakeAdapter) live(req PeerWakeRequest) bool {
	// A successful wake-agy invocation proves that agentapi accepted the
	// seat-addressed notification. That is the steady-state worker interrupt;
	// chat paste is not required for a live delivery. Coordinator notify remains
	// stamp-only until its MCP interrupt is applied by ApplyMCPLiveInterrupt.
	if req.SeatKind == SeatKindWorker {
		mode := strings.TrimSpace(req.DeliveryMode)
		return mode == datacell.DeliveryModeNotify || mode == datacell.DeliveryModePaste
	}
	return strings.TrimSpace(req.DeliveryMode) == datacell.DeliveryModePaste
}

// wakeScriptArgs picks membrane flags for delivery_mode (shell adapter only).
func wakeScriptArgs(deliveryMode, pasteText string) []string {
	switch strings.TrimSpace(deliveryMode) {
	case datacell.DeliveryModePaste:
		return []string{"--chat", pasteText}
	default:
		return []string{"--notify-only", pasteText}
	}
}

// wakeScriptArgsForTPM builds argv for the coordinator shell membrane.
func wakeScriptArgsForTPM(deliveryMode, message string) []string {
	msg := strings.TrimSpace(message)
	switch strings.TrimSpace(deliveryMode) {
	case datacell.DeliveryModePaste:
		return []string{"--paste-only", msg}
	default:
		return []string{"--stamp-only", msg}
	}
}
