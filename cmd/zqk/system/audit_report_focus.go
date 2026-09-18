package system

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// Audit report --focus values. The CLI flag has always accepted a
// "failures|timeouts|slow|all" value (see NewAuditReportCmd), but the
// value was never read; the full report was always rendered. That silently
// ignored input is the concrete CLI-churn symptom this file fixes:
// the flag now filters the analysis before report generation.
const (
	FocusFailures = "failures"
	FocusTimeouts = "timeouts"
	FocusSlow     = "slow"
	FocusChurn    = "churn"
	FocusAll      = "all"
)

// ApplyFocusFilter returns a copy of the analysis retaining only the
// sections matching the given --focus value. Empty and "all" are identity;
// unknown values fall back to "all" so a typo cannot erase the whole report
// (an extra source of confusion). nil analysis/"" focus return input as-is.
func ApplyFocusFilter(result *clipkg.AnalysisResult, focus string) *clipkg.AnalysisResult {
	if result == nil {
		return nil
	}
	switch focus {
	case "", FocusAll:
		return result
	case FocusFailures:
		out := *result
		out.FrequentTimeouts = nil
		out.SlowCommands = nil
		out.ChurnIndicators = nil
		return &out
	case FocusTimeouts:
		out := *result
		out.HighFailureRateCommands = nil
		out.SlowCommands = nil
		out.ChurnIndicators = nil
		return &out
	case FocusSlow:
		out := *result
		out.HighFailureRateCommands = nil
		out.FrequentTimeouts = nil
		out.ChurnIndicators = nil
		return &out
	case FocusChurn:
		out := *result
		out.HighFailureRateCommands = nil
		out.FrequentTimeouts = nil
		out.SlowCommands = nil
		return &out
	default:
		return result
	}
}
