package adapters

import (
	"context"

	"github.com/zqk-os/zqk/pkg/adapters/antigravity"
	"github.com/zqk-os/zqk/pkg/adapters/macos"
)

// MessageDelivery is the kernel-facing membrane for injecting a chat message
// into a vendor host. Implementations are vendor-specific; callers must not
// assume a filesystem layout or OS clipboard.
type MessageDelivery interface {
	Vendor() string
	Deliver(ctx context.Context, message string, msgID string, renderDetails map[string]string) error
}

// Resolve selects a vendor message adapter.
//
// Detection is delegated to each vendor package (Antigravity owns its brain
// transcript layout). When no vendor transcript is recognized, the macOS
// pbcopy/osascript paste adapter is the host fallback — it is not a
// kernel-native delivery path and fails closed off darwin.
func Resolve(transcriptPath string) MessageDelivery {
	if antigravity.IsBrainTranscript(transcriptPath) {
		return &antigravity.InboxAdapter{TranscriptPath: transcriptPath}
	}
	return &macos.ClipboardPasteAdapter{}
}
