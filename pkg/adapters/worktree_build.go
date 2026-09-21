package adapters

import "context"

// WorktreeBuildCheck is the pre-merge compile gate for an isolated ATK
// worktree. Toolchain adapters own compiler names and package layouts;
// cmd/zqk/agent must not invoke `go` or `./cmd/...`.
type WorktreeBuildCheck interface {
	Vendor() string
	Verify(ctx context.Context, worktreeRoot string) error
}
