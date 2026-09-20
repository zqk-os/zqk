package agent

import "github.com/zqk-os/zqk/pkg/adapters"

// MessageDeliveryAdapter is the kernel-facing message membrane.
// Concrete vendors live in pkg/adapters/<vendor> (Antigravity inbox, macOS paste).
type MessageDeliveryAdapter = adapters.MessageDelivery

// GetDeliveryAdapter selects a vendor message adapter.
// Path layout detection is owned by pkg/adapters/antigravity; this function
// does not inspect vendor directory names.
func GetDeliveryAdapter(transcriptPath string) MessageDeliveryAdapter {
	return adapters.Resolve(transcriptPath)
}
