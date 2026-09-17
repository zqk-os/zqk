package system

import (
	"fmt"
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/when"
	"golang.org/x/term"
)

// TierGroupedResults groups results by tier and visibility
type TierGroupedResults struct {
	Tier1         []CheckResult
	Tier2         []CheckResult
	Tier3         []CheckResult
	Tier4         []CheckResult
	Clean         []CheckResult
	PublicTier1   []CheckResult
	PublicTier2   []CheckResult
	PublicTier3   []CheckResult
	PublicTier4   []CheckResult
	PublicClean   []CheckResult
	InternalTier1 []CheckResult
	InternalTier2 []CheckResult
	InternalTier3 []CheckResult
	InternalTier4 []CheckResult
	InternalClean []CheckResult
}

// getTerminalWidth gets the terminal width, defaulting to 80
func getTerminalWidth() int {
	terminalWidth := 80
	if w, _, err := term.GetSize(0); err == nil && w > 0 {
		terminalWidth = w
	}
	return terminalWidth
}

// groupResultsByTier groups results by their highest tier issue
func groupResultsByTier(results []CheckResult) (tier1, tier2, tier3, tier4, clean []CheckResult) {
	for _, result := range results {
		hasTier1 := false
		hasTier2 := false
		hasTier3 := false
		hasTier4 := false

		for _, issue := range result.Issues {
			switch issue.Tier {
			case 1:
				hasTier1 = true
			case 2:
				hasTier2 = true
			case 3:
				hasTier3 = true
			case 4:
				hasTier4 = true
			}
		}

		when.When(func() bool { return hasTier1 }).Then(func() {
			tier1 = append(tier1, result)
		}).OrElseWhen(func() bool { return hasTier2 }).Then(func() {
			tier2 = append(tier2, result)
		}).OrElseWhen(func() bool { return hasTier3 }).Then(func() {
			tier3 = append(tier3, result)
		}).OrElseWhen(func() bool { return hasTier4 }).Then(func() {
			tier4 = append(tier4, result)
		}).OrElse(func() {
			clean = append(clean, result)
		}).Run()
	}
	return tier1, tier2, tier3, tier4, clean
}

// separatePublicAndInternal separates results into public and internal
func separatePublicAndInternal(grouped *TierGroupedResults) {
	grouped.PublicTier1, grouped.InternalTier1 = separateByVisibility(grouped.Tier1)
	grouped.PublicTier2, grouped.InternalTier2 = separateByVisibility(grouped.Tier2)
	grouped.PublicTier3, grouped.InternalTier3 = separateByVisibility(grouped.Tier3)
	grouped.PublicTier4, grouped.InternalTier4 = separateByVisibility(grouped.Tier4)
	grouped.PublicClean, grouped.InternalClean = separateByVisibility(grouped.Clean)
}

// separateByVisibility separates results into public and internal
func separateByVisibility(results []CheckResult) (public, internal []CheckResult) {
	for _, result := range results {
		when.When(func() bool { return isInternalKind(result.ObjectKind) }).Then(func() {
			internal = append(internal, result)
		}).OrElse(func() {
			public = append(public, result)
		}).Run()
	}
	return public, internal
}

// writePublicSummary writes the public objects summary
func writePublicSummary(buf *strings.Builder, grouped *TierGroupedResults, tierFilterActive bool, tierFilter int) {
	out := buf
	buf.WriteString("Public Objects:\n")
	when.When(func() bool { return tierFilterActive }).Then(func() {
		switch tierFilter {
		case 1:
			fmt.Fprintf(out, "  Tier 1 (Blocking): %d objects\n", len(grouped.PublicTier1))
		case 2:
			fmt.Fprintf(out, "  Tier 2 (Warnings): %d objects\n", len(grouped.PublicTier2))
		case 3:
			fmt.Fprintf(out, "  Tier 3 (Informational): %d objects\n", len(grouped.PublicTier3))
		case 4:
			fmt.Fprintf(out, "  Tier 4 (Recommendations): %d objects\n", len(grouped.PublicTier4))
		}
	}).OrElse(func() {
		fmt.Fprintf(out, "  Tier 1 (Blocking): %d objects\n", len(grouped.PublicTier1))
		fmt.Fprintf(out, "  Tier 2 (Warnings): %d objects\n", len(grouped.PublicTier2))
		fmt.Fprintf(out, "  Tier 3 (Informational): %d objects\n", len(grouped.PublicTier3))
		fmt.Fprintf(out, "  Tier 4 (Recommendations): %d objects\n", len(grouped.PublicTier4))
		fmt.Fprintf(out, "  Clean: %d objects\n\n", len(grouped.PublicClean))
	}).Run()
}

// writeInternalSummary writes the internal objects summary
func writeInternalSummary(buf *strings.Builder, grouped *TierGroupedResults, tierFilterActive bool, tierFilter int) {
	out := buf
	when.When(func() bool { return tierFilterActive }).Then(func() {
		var internalCount int
		switch tierFilter {
		case 1:
			internalCount = len(grouped.InternalTier1)
		case 2:
			internalCount = len(grouped.InternalTier2)
		case 3:
			internalCount = len(grouped.InternalTier3)
		case 4:
			internalCount = len(grouped.InternalTier4)
		}
		if internalCount > 0 {
			buf.WriteString("System Health (Internal Objects):\n")
			fmt.Fprintf(out, "  Tier %d: %d objects\n", tierFilter, internalCount)
			buf.WriteString("\n")
		}
	}).OrElse(func() {
		totalInternal := len(grouped.InternalTier1) + len(grouped.InternalTier2) + len(grouped.InternalTier3) + len(grouped.InternalTier4) + len(grouped.InternalClean)
		if totalInternal > 0 {
			buf.WriteString("System Health (Internal Objects):\n")
			if len(grouped.InternalTier1) > 0 {
				fmt.Fprintf(out, "  ⚠️  Blocking: %d (see details below)\n", len(grouped.InternalTier1))
			}
			if len(grouped.InternalTier2) > 0 {
				fmt.Fprintf(out, "  ⚠️  Warnings: %d (mostly hash mismatches - being fixed automatically)\n", len(grouped.InternalTier2))
			}
			if len(grouped.InternalTier3) > 0 {
				fmt.Fprintf(out, "  ℹ️  Informational: %d\n", len(grouped.InternalTier3))
			}
			fmt.Fprintf(out, "  ✓ Clean: %d objects\n", len(grouped.InternalClean))
			buf.WriteString("\n")
		}
	}).Run()
}

// writeTier1Details writes detailed Tier 1 issues
func writeTier1Details(buf *strings.Builder, grouped *TierGroupedResults, terminalWidth int, ctx *cli.Context) {
	out := buf
	var filteredPublic []CheckResult
	for _, r := range grouped.PublicTier1 {
		var filteredIssues []Issue
		for _, issue := range r.Issues {
			if issue.Tier == 1 {
				isCAS := issue.Category == categoryIntegrity || issue.Category == categoryRegistration
				if ctx.Verbose || isCAS {
					filteredIssues = append(filteredIssues, issue)
				}
			}
		}
		if len(filteredIssues) > 0 {
			rCopy := r
			rCopy.Issues = filteredIssues
			filteredPublic = append(filteredPublic, rCopy)
		}
	}

	if len(filteredPublic) > 0 {
		buf.WriteString("=== Tier 1: Blocking Issues (Public Objects) ===\n")
		for _, result := range filteredPublic {
			fmt.Fprintf(out, "\n%s (%s):\n", result.ObjectID, result.ObjectKind)
			for _, issue := range result.Issues {
				message := fmt.Sprintf("❌  [%s] %s", issue.Category, issue.Message)
				buf.WriteString(wrapText(message, terminalWidth, "  ") + "\n")
				if issue.FixCommand != emptyValue {
					buf.WriteString(wrapText("  → Fix: "+issue.FixCommand, terminalWidth, "  ") + "\n")
				}
			}
		}
		buf.WriteString("\n")
	}

	if len(grouped.InternalTier1) > 0 {
		writeInternalTier1Details(buf, grouped.InternalTier1, terminalWidth, ctx)
	}
}

// writeInternalTier1Details writes internal Tier 1 details
func writeInternalTier1Details(buf *strings.Builder, internalTier1 []CheckResult, terminalWidth int, ctx *cli.Context) {
	out := buf
	buf.WriteString("=== System Health (Internal Objects) ===\n")

	var filteredInternal []CheckResult
	for _, r := range internalTier1 {
		var filteredIssues []Issue
		for _, issue := range r.Issues {
			if issue.Tier == 1 {
				isCAS := issue.Category == categoryIntegrity || issue.Category == categoryRegistration
				if ctx.Verbose || isCAS {
					filteredIssues = append(filteredIssues, issue)
				}
			}
		}
		if len(filteredIssues) > 0 {
			rCopy := r
			rCopy.Issues = filteredIssues
			filteredInternal = append(filteredInternal, rCopy)
		}
	}

	fmt.Fprintf(out, "⚠️  %d blocking issue(s) detected in system objects\n", len(filteredInternal))

	issueCounts := make(map[string]int)
	for _, result := range filteredInternal {
		for _, issue := range result.Issues {
			issueCounts[issue.Category]++
		}
	}

	for category, count := range issueCounts {
		fmt.Fprintf(out, "  • %s: %d issue(s)\n", category, count)
	}

	when.When(func() bool { return ctx.Verbose || len(filteredInternal) <= 3 }).Then(func() {
		if len(filteredInternal) > 0 {
			buf.WriteString("\nDetails:\n")
			for _, result := range filteredInternal {
				fmt.Fprintf(out, "  %s (%s):\n", result.ObjectID, result.ObjectKind)
				for _, issue := range result.Issues {
					message := fmt.Sprintf("    ❌  [%s] %s", issue.Category, issue.Message)
					buf.WriteString(wrapText(message, terminalWidth, "      ") + "\n")
					if issue.FixCommand != emptyValue {
						buf.WriteString(wrapText("    → Fix: "+issue.FixCommand, terminalWidth, "      ") + "\n")
					}
				}
			}
		}
	}).OrElse(func() {
		if len(filteredInternal) > 0 {
			fmt.Fprintf(out, "\n  Use '%s system check --verbose' to see details\n", paths.CLICommandName)
		}
	}).Run()
	buf.WriteString("\n")
}

// writeTier2Details writes detailed Tier 2 warnings (verbose only)
func writeTier2Details(buf *strings.Builder, grouped *TierGroupedResults, terminalWidth int) {
	// Remove limits when verbose - show all violations for resolution
	if len(grouped.PublicTier2) > 0 {
		buf.WriteString("=== Tier 2: Warnings (Public Objects) ===\n")
		for _, result := range grouped.PublicTier2 {
			writeResultIssues(buf, result, 2, terminalWidth, "⚠️")
		}
		buf.WriteString("\n")
	}

	if len(grouped.InternalTier2) > 0 {
		buf.WriteString("=== Tier 2: Warnings (Internal Objects) ===\n")
		for _, result := range grouped.InternalTier2 {
			writeResultIssues(buf, result, 2, terminalWidth, "⚠️")
		}
		buf.WriteString("\n")
	}
}

// writeTier3Details writes detailed Tier 3 informational issues (verbose only)
func writeTier3Details(buf *strings.Builder, grouped *TierGroupedResults, terminalWidth int, ctx *cli.Context) {
	out := buf
	const maxInformational = 10

	if len(grouped.PublicTier3) > 0 {
		buf.WriteString("=== Tier 3: Informational (Public Objects) ===\n")
		shown := 0
		for _, result := range grouped.PublicTier3 {
			if shown >= maxInformational {
				fmt.Fprintf(out, "\n... and %d more informational issues (use '%s system check --tier 3' to see all)\n", len(grouped.PublicTier3)-maxInformational, paths.CLICommandName)
				break
			}
			writeResultIssues(buf, result, 3, terminalWidth, "ℹ️")
			shown++
		}
		buf.WriteString("\n")
	}

	if len(grouped.InternalTier3) > 0 {
		writeInternalTier3Details(buf, grouped.InternalTier3, terminalWidth, ctx)
	}
}

// writeInternalTier3Details writes internal Tier 3 informational issue details
func writeInternalTier3Details(buf *strings.Builder, internalTier3 []CheckResult, terminalWidth int, ctx *cli.Context) {
	out := buf
	buf.WriteString("=== Tier 3: Informational (Internal Objects) ===\n")
	fmt.Fprintf(out, "ℹ️  %d informational issue(s) in system objects\n\n", len(internalTier3))
	for _, result := range internalTier3 {
		writeResultIssues(buf, result, 3, terminalWidth, "ℹ️")
	}
	buf.WriteString("\n")
}

// writeResultIssues writes issues for a single result
func writeResultIssues(buf *strings.Builder, result CheckResult, tier int, terminalWidth int, icon string) {
	out := buf
	fmt.Fprintf(out, "\nObject ID: %s\n", result.ObjectID)
	fmt.Fprintf(out, "Kind: %s\n", result.ObjectKind)
	if result.FilePath != emptyValue {
		fmt.Fprintf(out, "File: %s\n", result.FilePath)
	}
	for _, issue := range result.Issues {
		if issue.Tier == tier {
			// Extract field name from message if present (common pattern: "Field <field> ...")
			fieldName := extractFieldName(issue.Message)
			if fieldName != emptyValue {
				fmt.Fprintf(out, "  Field: %s\n", fieldName)
			}
			fmt.Fprintf(out, "  Category: %s\n", issue.Category)
			fmt.Fprintf(out, "  Message: %s\n", issue.Message)
			if issue.FixCommand != emptyValue {
				fmt.Fprintf(out, "  Fix Command: %s\n", issue.FixCommand)
			}
			fmt.Fprintf(out, "  Auto-fixable: %v\n", issue.AutoFixable)
			buf.WriteString("\n")
		}
	}
}

// extractFieldName extracts field name from validation message
// Common patterns:
//   - "Field <field> is required"
//   - "Field <field> has invalid value"
//   - "Field <field> does not match pattern"
func extractFieldName(message string) string {
	// Pattern: "Field <field> ..."
	if strings.HasPrefix(message, "Field ") {
		parts := strings.Fields(message)
		if len(parts) >= 2 {
			field := strings.TrimSuffix(parts[1], ":")
			return field
		}
	}
	// Pattern: "<field> (Tier X): ..."
	if idx := strings.Index(message, " ("); idx > 0 {
		return message[:idx]
	}
	return ""
}

// writeDetailedStatistics writes detailed statistics (verbose only)
func writeDetailedStatistics(buf *strings.Builder, results []CheckResult, grouped *TierGroupedResults) {
	out := buf
	buf.WriteString("=== Detailed Statistics ===\n")
	// Clarify that system check only validates .zqk/process/ objects (not .zqk/ objects like audit_event, metrics)
	fmt.Fprintf(out, "Total objects checked: %d (from %s/ via object ID cache)\n", len(results), paths.ProcessDir)
	fmt.Fprintf(out, "  Public objects: %d\n", len(grouped.PublicTier1)+len(grouped.PublicTier2)+len(grouped.PublicTier3)+len(grouped.PublicTier4)+len(grouped.PublicClean))
	fmt.Fprintf(out, "  Internal objects: %d\n", len(grouped.InternalTier1)+len(grouped.InternalTier2)+len(grouped.InternalTier3)+len(grouped.InternalTier4)+len(grouped.InternalClean))
	fmt.Fprintf(out, "Objects with issues: %d\n", len(grouped.Tier1)+len(grouped.Tier2)+len(grouped.Tier3))
	fmt.Fprintf(out, "  Public: %d\n", len(grouped.PublicTier1)+len(grouped.PublicTier2)+len(grouped.PublicTier3))
	fmt.Fprintf(out, "  Internal: %d\n", len(grouped.InternalTier1)+len(grouped.InternalTier2)+len(grouped.InternalTier3))
	fmt.Fprintf(out, "Clean objects: %d\n", len(grouped.Clean))
	fmt.Fprintf(out, "  Public: %d\n", len(grouped.PublicClean))
	fmt.Fprintf(out, "  Internal: %d\n", len(grouped.InternalClean))
	if len(grouped.Tier1) > 0 {
		fmt.Fprintf(out, "Blocking issues: %d (public: %d, internal: %d)\n", len(grouped.Tier1), len(grouped.PublicTier1), len(grouped.InternalTier1))
	}
	if len(grouped.Tier2) > 0 {
		fmt.Fprintf(out, "Warnings: %d (public: %d, internal: %d)\n", len(grouped.Tier2), len(grouped.PublicTier2), len(grouped.InternalTier2))
	}
	if len(grouped.Tier3) > 0 {
		fmt.Fprintf(out, "Informational: %d (public: %d, internal: %d)\n", len(grouped.Tier3), len(grouped.PublicTier3), len(grouped.InternalTier3))
	}

	// Display object ID cache information
	cache := GetGlobalObjectIDCache()
	cacheEntryCount := cache.GetEntryCount()
	if cacheEntryCount > 0 {
		fmt.Fprintf(out, "\nObject ID Cache: %d entries\n", cacheEntryCount)
		metadata := cache.GetMetadata()
		if metadata != nil && !metadata.BuildTime.IsZero() {
			fmt.Fprintf(out, "  Cache built: %s\n", metadata.BuildTime.Format("2006-01-02 15:04:05"))
		}
	}

	buf.WriteString("\n")
}

// writeAutoFixedItems writes auto-fixed items
func writeAutoFixedItems(buf *strings.Builder, results []CheckResult, terminalWidth int) {
	out := buf
	autoFixedCount := 0
	for _, result := range results {
		if len(result.AutoFixed) > 0 {
			autoFixedCount++
		}
	}

	if autoFixedCount > 0 {
		buf.WriteString("=== Auto-Fixed Issues ===\n")
		for _, result := range results {
			if len(result.AutoFixed) > 0 {
				fmt.Fprintf(out, "\n%s (%s):\n", result.ObjectID, result.ObjectKind)
				for _, fixed := range result.AutoFixed {
					message := fmt.Sprintf("✅  %s", fixed)
					buf.WriteString(wrapText(message, terminalWidth, "  ") + "\n")
				}
			}
		}
		buf.WriteString("\n")
	}
}

// writeResolutionSummary writes a short resolution block when check --auto-fix ran
// (Stale CAS cleanup and/or auto-fixed counts) per INTEGRITY_RESOLUTION_PLAN.
func writeResolutionSummary(buf *strings.Builder, results []CheckResult, staleCASResult *StaleCASCleanupResult, terminalWidth int) {
	out := buf
	autoFixedCount := 0
	for _, r := range results {
		autoFixedCount += len(r.AutoFixed)
	}
	showStaleCAS := staleCASResult != nil && (len(staleCASResult.KindsRun) > 0 || staleCASResult.FilesHandled > 0)
	if autoFixedCount == 0 && !showStaleCAS {
		return
	}
	buf.WriteString("=== Resolution ===\n")
	if autoFixedCount > 0 {
		fmt.Fprintf(out, "Auto-fixed: %d issue(s).\n", autoFixedCount)
	}
	if showStaleCAS {
		kindsStr := strings.Join(staleCASResult.KindsRun, ", ")
		fmt.Fprintf(out, "Stale CAS cleanup: ran for kind(s) %s (%d file(s) quarantined).\n", kindsStr, staleCASResult.FilesHandled)
	}
	buf.WriteString("\n")
}

// writeAuditEventBufferStatus writes audit event buffer status
func writeAuditEventBufferStatus(buf *strings.Builder, bufferCount int, bufferSummary map[string]int) {
	out := buf
	if bufferCount > 0 {
		buf.WriteString("=== Audit Events ===\n")
		fmt.Fprintf(out, "⚠️  Note: %d audit event(s) are being buffered and will be written/flushed after this operation completes.\n", bufferCount)
		if len(bufferSummary) > 0 {
			buf.WriteString("   Buffered events by type:\n")
			for eventType, count := range bufferSummary {
				fmt.Fprintf(out, "     - %s: %d occurrence(s)\n", eventType, count)
			}
		}
		buf.WriteString("   Run the command again to see final audit event counts.\n\n")
	}
}
