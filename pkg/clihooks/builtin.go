package clihooks

// ProtocolVersion and hook IDs: external automation contract (docs: CLI_EXTERNAL_HOOK_PROTOCOL.md).
// Bump ProtocolVersion when documented CLI behavior or hook IDs change. Distinct from JSON "version" in cli_hook_profile.json.
const (
	ProtocolVersion = "zqk.cli_hooks.v1"

	// Built-in hook IDs (stable contract for git hooks, CI, and tray automation).
	// HookPostCommitScanTests runs background test bundles for packages changed in the last commit.
	// Default implementation: tools/git-hooks/post-commit → scripts/run-tests-for-changed-packages.sh
	// See docs/architecture/PRE_COMMIT_BACKGROUND_RESULTS.md
	HookPostCommitScanTests = "post_commit_scan_tests"
)

func builtinHooks() []*Hook {
	return []*Hook{
		{
			ID:          HookPostCommitScanTests,
			Description: "After each commit, run scheduler scan-tests for packages with changed .go files (background)",
			Enabled:     true,
			TrayEntry:   "", // optional: non-empty means prefer `zqk tray run <name>` for this hook
		},
	}
}
