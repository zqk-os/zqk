package adapters

import (
	"context"

	"github.com/zqk-os/zqk/pkg/swarm"
)

// CompletionGate is the kernel-facing membrane for seat-worker compile/test
// evidence. Implementations are toolchain-specific (Go, later Python, …);
// cmd/zqk/agent must not hard-code language suffixes or package layouts.
type CompletionGate interface {
	Vendor() string
	WrittenFiles(history []swarm.ToolCallRecord, root string) []string
	Verify(ctx context.Context, root string, history []swarm.ToolCallRecord) (feedback string, err error)
}
