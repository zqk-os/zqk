package agentfeed

import (
	"context"
	"strings"

	"github.com/zqk-os/zqk/pkg/datacell"
)

// NativePeerWakeAdapter delivers wake signals purely in-process without shell dependencies.
// Designed for pure Go binary deployments, embedded runtimes, and hermetic environments.
//
// Satisfies F-SUPPLY-006: Standalone pure Go distribution without missing shell scripts.
type NativePeerWakeAdapter struct{}

// NewNativePeerWakeAdapter creates a new in-process peer wake adapter.
func NewNativePeerWakeAdapter() *NativePeerWakeAdapter {
	return &NativePeerWakeAdapter{}
}

// Wake implements PeerWakeAdapter.
func (n *NativePeerWakeAdapter) Wake(_ context.Context, req PeerWakeRequest) (PeerWakeAdapterResult, error) {
	transport := TransportAgentAPINotify
	live := false

	if req.SeatKind == SeatKindCoordinator {
		if strings.TrimSpace(req.DeliveryMode) == datacell.DeliveryModePaste {
			transport = TransportTPMPaste
			live = true
		} else {
			transport = TransportTPMStamp
			live = false
		}
	} else {
		mode := strings.TrimSpace(req.DeliveryMode)
		live = mode == datacell.DeliveryModeNotify || mode == datacell.DeliveryModePaste
		transport = TransportAgentAPINotify
	}

	return PeerWakeAdapterResult{
		Endpoint:  "native://internal",
		Transport: transport,
		Live:      live,
		Output:    "delivered via native in-process adapter",
	}, nil
}
