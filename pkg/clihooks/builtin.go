package clihooks

import "github.com/zqk-os/zqk/pkg/paths"

// ProtocolVersion and hook IDs: external automation contract (docs: CLI_EXTERNAL_HOOK_PROTOCOL.md).
// Bump ProtocolVersion when documented CLI behavior or hook IDs change. Distinct from JSON "version" in cli_hook_profile.json.
const (
	ProtocolVersion = "zqk.cli_hooks.v1"

	// Built-in hook IDs (stable contract for git hooks, CI, and tray automation).
	// HookPostCommitScanTests is a stable hook ID (do not rename).
	// After commit, discover/bind/run kernel test_case objects for changed packages.
	HookPostCommitScanTests = "post_commit_scan_tests"
)

func builtinHooks() []*Hook {
	return []*Hook{
		{
			ID:          HookPostCommitScanTests,
			Description: paths.RewriteCanonicalCLIInvocations("After each commit, run zqk test discover / zqk test run for packages with changed .go files"),
			Enabled:     true,
			TrayEntry:   "", // optional: non-empty means prefer `zqk tray run <name>` for this hook
		},
	}
}
