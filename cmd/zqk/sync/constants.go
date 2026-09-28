package sync

const (
	FlagRepo      = "repo"
	FlagDryRun    = "dry-run"
	FlagDirection = "direction"
	FlagToken     = "token"
	FlagTeam      = "team"
	FlagAPIKey    = "api-key"

	EnvGitHubToken  = "GITHUB_TOKEN"
	EnvLinearAPIKey = "LINEAR_API_KEY"

	ErrStorageUnavailable = "storage unavailable"

	MsgDryRunGitHubFmt        = "⚡ [DRY-RUN] GitHub Issue synchronization simulation for repository: %s\n"
	MsgDryRunLinearFmt        = "⚡ [DRY-RUN] Linear Ticket synchronization simulation for team: %s\n"
	MsgDryRunDirectionFmt     = "  Direction: %s | Target: Knowledge Kernel CAS\n"
	MsgDryRunVerified         = "✓ Verified connection and payload schema compatibility (0 conflicts).\n"
	MsgErrGitHubTokenRequired = "github token required (pass --token or set GITHUB_TOKEN environment variable)"
	MsgErrLinearAPIKeyRequired = "linear api-key required (pass --api-key or set LINEAR_API_KEY environment variable)"
	MsgGitHubSyncCompletedFmt = "✓ GitHub synchronization completed for %s (direction: %s)\n"
	MsgLinearSyncCompletedFmt = "✓ Linear synchronization completed for team %s (direction: %s)\n"

	MsgSyncStatusHeader    = "Knowledge Kernel External Issue Sync Status:\n"
	MsgSyncStatusGitHubFmt = "  GitHub Issues Synchronized: %d\n"
	MsgSyncStatusLinearFmt = "  Linear Tickets Synchronized: %d\n"
	MsgSyncStatusTotalFmt  = "  Total Synced Work Units:    %d\n"

	PrefixBliGitHub = "BLI-GH-"
	PrefixBliLinear = "BLI-LIN-"
)
