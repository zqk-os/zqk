package eval

import (
	"fmt"
	"sort"
	"strings"
)

// DriftTurnObservation records the invariant check outcome for a single agent turn
// in a multi-turn compaction drift benchmark (Study 3: CLAUDE.md vs Fail-Closed Gates).
type DriftTurnObservation struct {
	SessionID          string   `json:"session_id"`
	Arm                string   `json:"arm"`
	Turn               int      `json:"turn"`
	ViolatedInvariants []string `json:"violated_invariants,omitempty"`
	BlockedByGate      bool     `json:"blocked_by_gate"`
	ClaimedDone        bool     `json:"claimed_done"`
	RecoveryRetries    int      `json:"recovery_retries"`
	GateOverheadTokens int      `json:"gate_overhead_tokens"`
}

// DriftTurnBucketSummary summarizes violation rates for a contiguous turn range (e.g. 1-10).
type DriftTurnBucketSummary struct {
	Label           string  `json:"label"`
	StartTurn       int     `json:"start_turn"`
	EndTurn         int     `json:"end_turn"`
	TotalTurns      int     `json:"total_turns"`
	ViolatedTurns   int     `json:"violated_turns"`
	ViolationRatePc float64 `json:"violation_rate_pct"`
}

// DriftArmSummary aggregates Study 3 compaction-drift and done-gate metrics for one experimental arm.
type DriftArmSummary struct {
	Arm                   string                   `json:"arm"`
	Sessions              int                      `json:"sessions"`
	TotalTurns            int                      `json:"total_turns"`
	TotalViolations       int                      `json:"total_violations"`
	BlockedViolations     int                      `json:"blocked_violations"`
	SilentFalseDoneCount  int                      `json:"silent_false_done_count"`
	SilentFalseDoneRatePc float64                  `json:"silent_false_done_rate_pct"`
	PrePRCatchRatePc      float64                  `json:"pre_pr_catch_rate_pct"`
	AvgRecoveryRetries    float64                  `json:"avg_recovery_retries"`
	AvgGateOverheadTokens float64                  `json:"avg_gate_overhead_tokens"`
	Buckets               []DriftTurnBucketSummary `json:"buckets"`
}

// DriftBenchmarkReport holds deterministic cross-arm Study 3 compaction drift results.
type DriftBenchmarkReport struct {
	BucketSize int               `json:"bucket_size"`
	MaxTurns   int               `json:"max_turns"`
	Arms       []DriftArmSummary `json:"arms"`
}

const (
	defaultDriftBucketSize = 10
	defaultDriftMaxTurns   = 40
	percentScale           = 100.0
)

// SummarizeDriftBenchmark computes deterministic per-arm drift curves, silent false-done rates,
// pre-PR catch rates, and honest gate overhead across all recorded turn observations.
func SummarizeDriftBenchmark(observations []DriftTurnObservation, bucketSize, maxTurns int) DriftBenchmarkReport {
	if bucketSize <= 0 {
		bucketSize = defaultDriftBucketSize
	}
	if maxTurns <= 0 {
		maxTurns = defaultDriftMaxTurns
	}

	byArm := make(map[string][]DriftTurnObservation)
	for _, obs := range observations {
		arm := strings.TrimSpace(obs.Arm)
		if arm == "" {
			arm = "unknown"
		}
		byArm[arm] = append(byArm[arm], obs)
	}

	armNames := make([]string, 0, len(byArm))
	for arm := range byArm {
		armNames = append(armNames, arm)
	}
	sort.Strings(armNames)

	report := DriftBenchmarkReport{
		BucketSize: bucketSize,
		MaxTurns:   maxTurns,
		Arms:       make([]DriftArmSummary, 0, len(armNames)),
	}

	for _, arm := range armNames {
		report.Arms = append(report.Arms, summarizeDriftArm(arm, byArm[arm], bucketSize, maxTurns))
	}
	return report
}

func summarizeDriftArm(arm string, obs []DriftTurnObservation, bucketSize, maxTurns int) DriftArmSummary {
	numBuckets := (maxTurns + bucketSize - 1) / bucketSize
	buckets := make([]DriftTurnBucketSummary, numBuckets)
	for i := range numBuckets {
		start := i*bucketSize + 1
		end := (i + 1) * bucketSize
		if end > maxTurns {
			end = maxTurns
		}
		buckets[i] = DriftTurnBucketSummary{
			Label:     fmt.Sprintf("%d-%d", start, end),
			StartTurn: start,
			EndTurn:   end,
		}
	}

	sessions := make(map[string]struct{})
	silentFalseDoneSessions := make(map[string]struct{})
	totalViolations := 0
	blockedViolations := 0
	totalRetries := 0
	totalOverheadTokens := 0

	for _, o := range obs {
		sessionKey := strings.TrimSpace(o.SessionID)
		if sessionKey == "" {
			sessionKey = "default"
		}
		sessions[sessionKey] = struct{}{}

		hasViolation := len(o.ViolatedInvariants) > 0
		if hasViolation {
			totalViolations++
			if o.BlockedByGate {
				blockedViolations++
			}
			if o.ClaimedDone && !o.BlockedByGate {
				silentFalseDoneSessions[sessionKey] = struct{}{}
			}
		}

		totalRetries += o.RecoveryRetries
		totalOverheadTokens += o.GateOverheadTokens

		if o.Turn >= 1 && o.Turn <= maxTurns {
			idx := (o.Turn - 1) / bucketSize
			if idx >= 0 && idx < len(buckets) {
				buckets[idx].TotalTurns++
				if hasViolation {
					buckets[idx].ViolatedTurns++
				}
			}
		}
	}

	for i := range buckets {
		if buckets[i].TotalTurns > 0 {
			buckets[i].ViolationRatePc = float64(buckets[i].ViolatedTurns) / float64(buckets[i].TotalTurns) * percentScale
		}
	}

	sessionCount := len(sessions)
	summary := DriftArmSummary{
		Arm:                  arm,
		Sessions:             sessionCount,
		TotalTurns:           len(obs),
		TotalViolations:      totalViolations,
		BlockedViolations:    blockedViolations,
		SilentFalseDoneCount: len(silentFalseDoneSessions),
		Buckets:              buckets,
	}

	if sessionCount > 0 {
		summary.SilentFalseDoneRatePc = float64(len(silentFalseDoneSessions)) / float64(sessionCount) * percentScale
		summary.AvgRecoveryRetries = float64(totalRetries) / float64(sessionCount)
		summary.AvgGateOverheadTokens = float64(totalOverheadTokens) / float64(sessionCount)
	}
	if totalViolations > 0 {
		summary.PrePRCatchRatePc = float64(blockedViolations) / float64(totalViolations) * percentScale
	}
	return summary
}

// FormatDriftCurveCSV renders per-arm turn-bucket violation rates as deterministic CSV
// matching Study 3's results/drift_by_turn.csv schema.
func FormatDriftCurveCSV(report DriftBenchmarkReport) string {
	var b strings.Builder
	b.WriteString("arm,turn_bucket,total_turns,violated_turns,violation_rate_pct\n")
	for _, arm := range report.Arms {
		for _, bucket := range arm.Buckets {
			fmt.Fprintf(&b, "%s,%s,%d,%d,%.2f\n",
				arm.Arm, bucket.Label, bucket.TotalTurns, bucket.ViolatedTurns, bucket.ViolationRatePc)
		}
	}
	return b.String()
}

// FormatDriftReportMarkdown renders a GitHub-Flavored Markdown comparison table
// across all experimental arms for PR review and benchmark publication artifacts.
func FormatDriftReportMarkdown(report DriftBenchmarkReport) string {
	var b strings.Builder
	b.WriteString("## Study 3 — Compaction Drift & Fail-Closed Done-Gate Report\n\n")
	b.WriteString("| Arm | Sessions | Turns | Violations | Pre-PR Catch (%) | Silent False-Done (%) | Avg Retries | Avg Gate Overhead (tokens) |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, arm := range report.Arms {
		fmt.Fprintf(&b, "| `%s` | %d | %d | %d | %.2f%% | %.2f%% | %.2f | %.0f |\n",
			arm.Arm,
			arm.Sessions,
			arm.TotalTurns,
			arm.TotalViolations,
			arm.PrePRCatchRatePc,
			arm.SilentFalseDoneRatePc,
			arm.AvgRecoveryRetries,
			arm.AvgGateOverheadTokens,
		)
	}
	return b.String()
}
