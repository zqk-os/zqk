package interactionpolicy

// Hint string constants — single source of truth for feed/ambient hint text.
//
// These literals were previously inlined in the Hint* builder functions in
// ambient_drive.go. Consolidating them here (DRY) lets callers and tests
// reference the same tokens without duplicating wording, and makes drift
// detection trivial: any new hint text MUST be registered here.
const (
	HintAlignRefreshText    = "align refresh"
	HintDraftClassifyText   = "draft classify"
	HintHourglassText       = "hourglass triage"
	HintTracePipelineText   = "trace pipeline"
	HintOrchestratePrefix   = "orchestrate plan "
	HintOrchestrateSuffix   = " — lead executing"
	HintObjectGetPrefix     = "object get "
	HintObjectGetSuffix     = " (resolve body before acting)"
	HintSwarmInitText       = "swarm init"
)

// IsKnownHint reports whether literal is one of the registered hint constants.
// Useful for feed parsers to distinguish hint text from free-form status prose.
func IsKnownHint(literal string) bool {
	switch literal {
	case HintAlignRefreshText,
		HintDraftClassifyText,
		HintHourglassText,
		HintTracePipelineText,
		HintOrchestratePrefix,
		HintOrchestrateSuffix,
		HintObjectGetPrefix,
		HintObjectGetSuffix,
		HintSwarmInitText:
		return true
	}
	return false
}
