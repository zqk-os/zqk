package validation

import "github.com/lanceman/zqk/pkg/shovelready"

// Canonical criteria id for CRI-SHOVEL-READY (MMORCH audit F-002).
// TRACK: CRIT-REDACTED — keep in sync with kernel criteria object.
const CriteriaIDShovelReady = shovelready.CriteriaID

// ShovelReadyResult is the CRI-SHOVEL-READY gate outcome for one backlog item.
type ShovelReadyResult = shovelready.Result

// EvaluateShovelReady checks MMORCH CRI-SHOVEL-READY against a backlog_item map.
func EvaluateShovelReady(bli map[string]any) ShovelReadyResult {
	return shovelready.Evaluate(bli)
}

// IsShovelReady is the boolean form of EvaluateShovelReady.
func IsShovelReady(bli map[string]any) bool {
	return shovelready.IsReady(bli)
}
