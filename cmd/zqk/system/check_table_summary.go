package system

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	pkgcli "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/objects"
)

// Objects that have not crossed the membrane are not layered — see writeObjectDraftPlaneSummary.
func writeLayeredSummary(buf *strings.Builder, results []CheckResult, cmd *cobra.Command) {
	if checkFastModeEnabled(cmd) {
		buf.WriteString(checkFastPartialVerdict)
		buf.WriteString("\n\n")
	}
	var layer0Issues []CheckResult
	var activeIssues []CheckResult
	var readyObjects []CheckResult
	var completeObjects []CheckResult
	var staticObjects []CheckResult
	includeIds, _ := cmd.Flags().GetBool("include-ids")
	detailsFlag, _ := cmd.Flags().GetBool("details")
	details := includeIds || detailsFlag

	for _, r := range results {
		status := strings.ToLower(r.Status)
		status = strings.ReplaceAll(status, " ", "_")
		isTerminal := false
		if r.Status != "" && r.ObjectKind != "" {
			isTerminal, _ = objects.GetGlobalLifecycleLoader().IsTerminalStatusForKind(r.ObjectKind, r.Status)
		}

		// Treat "error" or "failed" status as a lifecycle blocker issue
		if status == "error" || status == "failed" {
			hasErrorStatusIssue := false
			for _, issue := range r.Issues {
				if strings.Contains(issue.Message, "status is") {
					hasErrorStatusIssue = true
					break
				}
			}
			if !hasErrorStatusIssue {
				r.Issues = append(r.Issues, Issue{
					Category: "lifecycle",
					Message:  fmt.Sprintf("Object is in %q status", r.Status),
					Tier:     1,
				})
			}
		}

		if len(r.Issues) == 0 {
			if r.Status == "" || isStaticConfigKind(r.ObjectKind) {
				staticObjects = append(staticObjects, r)
			} else if isTerminal {
				completeObjects = append(completeObjects, r)
			} else {
				if issue, isPreliminary := preliminaryCASIssue(r); isPreliminary {
					r.Issues = append(r.Issues, issue)
					layer0Issues = append(layer0Issues, r)
				} else {
					readyObjects = append(readyObjects, r)
				}
			}
		} else {
			// Check if there are any Layer 0 (CAS/Integrity) issues
			hasCASIssue := false
			for _, issue := range r.Issues {
				if issue.Category == categoryIntegrity || issue.Category == categoryRegistration {
					hasCASIssue = true
					break
				}
			}

			if hasCASIssue {
				layer0Issues = append(layer0Issues, r)
			} else {
				// Layer 1 = any in-membrane object with non-CAS issues. Do not require
				// status∈{active,in_progress,planned}: identified/exploring/complete (and empty
				// status from incomplete metadata) still need fixes. A preliminary status inside
				// CAS means the membrane crossing was illegitimate, so it belongs to Layer 0.
				if issue, isPreliminary := preliminaryCASIssue(r); isPreliminary {
					r.Issues = append(r.Issues, issue)
					layer0Issues = append(layer0Issues, r)
				} else {
					activeIssues = append(activeIssues, r)
				}
			}
		}
	}

	bold := color.New(color.Bold).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	buf.WriteString(fmt.Sprintf("\n%s\n\n",
		bold(cyan("=== System Check: High-Level Layered Summary ===")),
	))

	// Layer 0: CAS & Integrity Blockers
	{
		headers := []string{"Issue", "Severity", "Kind"}
		var rows [][]string
		if len(layer0Issues) > 0 {
			rows = getGroupedLayer0Rows(layer0Issues, details)
		} else {
			rows = append(rows, []string{green("✓ No CAS or integrity blockers found."), "-", "-"})
		}
		buf.WriteString(renderTableWithTitle("Layer 0: CAS & Integrity Blockers", headers, rows))
		buf.WriteString("\n")
	}

	// Layer 1: CAS objects with non-integrity validation issues.
	{
		headers := []string{"Issue", "Severity", "Kind"}
		var rows [][]string
		if len(activeIssues) > 0 {
			rows = getGroupedIssueRows(activeIssues, details)
		} else {
			rows = append(rows, []string{green("✓ No CAS object issues found."), "-", "-"})
		}
		buf.WriteString(renderTableWithTitle("Layer 1: Objects Needing Fixes (Inside CAS Membrane)", headers, rows))
		buf.WriteString("\n")
	}

	// Layer 2: Ready for Execution/Promotion
	{
		headers := []string{"Kind", "Ready Count", "Status Breakdown"}
		var rows [][]string
		if len(readyObjects) > 0 {
			byKind := make(map[string][]CheckResult)
			for _, r := range readyObjects {
				byKind[r.ObjectKind] = append(byKind[r.ObjectKind], r)
			}
			var kinds []string
			for k := range byKind {
				kinds = append(kinds, k)
			}
			sort.Strings(kinds)

			for _, kind := range kinds {
				objs := byKind[kind]
				statusCounts := make(map[string]int)
				for _, r := range objs {
					status := strings.ToLower(r.Status)
					if status == "" {
						status = "unknown"
					}
					statusCounts[status]++
				}
				var parts []string
				var statuses []string
				for s := range statusCounts {
					statuses = append(statuses, s)
				}
				sort.Strings(statuses)
				for _, s := range statuses {
					cnt := statusCounts[s]
					sColored := s
					switch s {
					case "active", "in_progress", "metrics_captured", "originated", "planned":
						sColored = yellow(s)
					case "approved", "ready":
						sColored = cyan(s)
					}
					parts = append(parts, fmt.Sprintf("%d %s", cnt, sColored))
				}
				statusStr := strings.Join(parts, ", ")
				rows = append(rows, []string{formatColumnHeader(kind), fmt.Sprintf("%d", len(objs)), statusStr})
			}
		} else {
			rows = append(rows, []string{"-", "-", "No objects ready for execution or promotion."})
		}
		buf.WriteString(renderTableWithTitle("Layer 2: Objects Ready for Execution/Promotion", headers, rows))
		buf.WriteString("\n")
	}

	// Layer 3: Completed Items
	{
		headers := []string{"Kind", "Completed Count", "Status Breakdown"}
		var rows [][]string
		if len(completeObjects) > 0 {
			byKind := make(map[string][]CheckResult)
			for _, r := range completeObjects {
				byKind[r.ObjectKind] = append(byKind[r.ObjectKind], r)
			}
			var kinds []string
			for k := range byKind {
				kinds = append(kinds, k)
			}
			sort.Strings(kinds)

			for _, kind := range kinds {
				objs := byKind[kind]
				statusCounts := make(map[string]int)
				for _, r := range objs {
					status := strings.ToLower(r.Status)
					if status == "" {
						status = "unknown"
					}
					statusCounts[status]++
				}
				var parts []string
				var statuses []string
				for s := range statusCounts {
					statuses = append(statuses, s)
				}
				sort.Strings(statuses)
				for _, s := range statuses {
					cnt := statusCounts[s]
					sColored := s
					switch s {
					case "complete", "implemented", "success", "resolved", "validated", "verified":
						sColored = green(s)
					case "archived", "deprecated":
						sColored = dim(s)
					case "deferred", "rejected":
						sColored = yellow(s)
					}
					parts = append(parts, fmt.Sprintf("%d %s", cnt, sColored))
				}
				statusStr := strings.Join(parts, ", ")
				rows = append(rows, []string{formatColumnHeader(kind), fmt.Sprintf("%d", len(objs)), statusStr})
			}
		} else {
			rows = append(rows, []string{"-", "-", "No completed objects found in this check run."})
		}
		buf.WriteString(renderTableWithTitle("Layer 3: Completed and Validated Items", headers, rows))
		buf.WriteString("\n")
	}

	// Static Configuration Health
	if len(staticObjects) > 0 {
		byKind := make(map[string]int)
		for _, r := range staticObjects {
			byKind[r.ObjectKind]++
		}
		var kinds []string
		for k := range byKind {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)

		var headers = []string{"Kind", "Compliant Count"}
		var rows [][]string
		for _, kind := range kinds {
			rows = append(rows, []string{formatColumnHeader(kind), fmt.Sprintf("%d", byKind[kind])})
		}
		buf.WriteString(renderTableWithTitle("Static configurations and specifications", headers, rows))
		buf.WriteString("\n")
	}
}

func getGroupedLayer0Rows(results []CheckResult, details bool) [][]string {
	type groupKey struct {
		Kind    string
		Message string
	}
	type groupInfo struct {
		Tier        int
		AutoFixable bool
		IDs         []string
	}

	groups := make(map[groupKey]*groupInfo)
	var keys []groupKey

	for _, r := range results {
		for _, issue := range r.Issues {
			if issue.Category == categoryIntegrity || issue.Category == categoryRegistration {
				normMsg := normalizeErrorMessage(issue.Message)
				key := groupKey{
					Kind:    r.ObjectKind,
					Message: normMsg,
				}
				info, exists := groups[key]
				if !exists {
					info = &groupInfo{
						Tier:        issue.Tier,
						AutoFixable: issue.AutoFixable,
					}
					groups[key] = info
					keys = append(keys, key)
				}
				info.IDs = append(info.IDs, r.ObjectID)
				if issue.AutoFixable {
					info.AutoFixable = true
				}
			}
		}
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Message != keys[j].Message {
			return keys[i].Message < keys[j].Message
		}
		return keys[i].Kind < keys[j].Kind
	})

	var rows [][]string
	var lastMessage string
	for _, key := range keys {
		info := groups[key]
		severitySymbol := "🟡"
		if info.Tier == 1 {
			severitySymbol = "❌"
		} else if info.Tier == 3 {
			severitySymbol = "🔹"
		} else if info.Tier == 4 {
			severitySymbol = "💡"
		}

		fixableStr := ""
		if info.AutoFixable {
			fixableStr = "  🔧 [Auto-fix]"
		}

		var suffix string
		if details {
			suffix = fixableStr + fmt.Sprintf(" (IDs: %s)", strings.Join(info.IDs, ", "))
		} else {
			suffix = fixableStr
		}

		msgText := key.Message + suffix
		displayMsg := msgText
		displaySeverity := severitySymbol
		if displayMsg == lastMessage {
			displayMsg = ""
			displaySeverity = ""
		} else {
			lastMessage = displayMsg
		}

		kindVal := fmt.Sprintf("%s (%d)", formatColumnHeader(key.Kind), len(info.IDs))
		rows = append(rows, []string{displayMsg, displaySeverity, kindVal})
	}
	return rows
}

func getGroupedIssueRows(results []CheckResult, details bool) [][]string {
	type groupKey struct {
		Kind    string
		Status  string
		Message string
	}
	type groupInfo struct {
		Tier        int
		AutoFixable bool
		IDs         []string
	}

	groups := make(map[groupKey]*groupInfo)
	var keys []groupKey

	for _, r := range results {
		for _, issue := range r.Issues {
			normMsg := normalizeErrorMessage(issue.Message)
			key := groupKey{
				Kind:    r.ObjectKind,
				Status:  r.Status,
				Message: normMsg,
			}
			info, exists := groups[key]
			if !exists {
				info = &groupInfo{
					Tier:        issue.Tier,
					AutoFixable: issue.AutoFixable,
				}
				groups[key] = info
				keys = append(keys, key)
			}
			info.IDs = append(info.IDs, r.ObjectID)
			if issue.AutoFixable {
				info.AutoFixable = true
			}
		}
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Message != keys[j].Message {
			return keys[i].Message < keys[j].Message
		}
		return keys[i].Kind < keys[j].Kind
	})

	var rows [][]string
	var lastMessage string
	for _, key := range keys {
		info := groups[key]
		severitySymbol := "🟡"
		if info.Tier == 1 {
			severitySymbol = "❌"
		} else if info.Tier == 3 {
			severitySymbol = "🔹"
		} else if info.Tier == 4 {
			severitySymbol = "💡"
		}

		fixableStr := ""
		if info.AutoFixable {
			fixableStr = "  🔧 [Auto-fix]"
		}

		var suffix string
		if details {
			suffix = fixableStr + fmt.Sprintf(" (IDs: %s)", strings.Join(info.IDs, ", "))
		} else {
			suffix = fixableStr
		}

		var kindVal string
		if key.Status != "" && !strings.Contains(strings.ToLower(key.Message), strings.ToLower(key.Status)) {
			kindVal = fmt.Sprintf("%s (%d, status: %q)", formatColumnHeader(key.Kind), len(info.IDs), key.Status)
		} else {
			kindVal = fmt.Sprintf("%s (%d)", formatColumnHeader(key.Kind), len(info.IDs))
		}

		msgText := key.Message + suffix
		displayMsg := msgText
		displaySeverity := severitySymbol
		if displayMsg == lastMessage {
			displayMsg = ""
			displaySeverity = ""
		} else {
			lastMessage = displayMsg
		}

		rows = append(rows, []string{displayMsg, displaySeverity, kindVal})
	}
	return rows
}

func renderTableWithTitle(title string, headers []string, rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}

	// Determine optimal column widths to sum up to exactly:
	// - 93 for 2 columns (overhead = 7, total = 100)
	// - 90 for 3 columns (overhead = 10, total = 100)
	widths := make([]int, len(headers))
	if len(headers) == 2 {
		// Static configurations
		widths[0] = 30
		widths[1] = 63
	} else if len(headers) == 3 {
		if headers[0] == "ISSUE" || headers[0] == "Issue" {
			// Layer 0 and Layer 1: Issue, Severity, Kind
			widths[0] = 55 // Issue message
			widths[1] = 10 // Severity icon
			widths[2] = 25 // Kind (count, status)
		} else if headers[1] == "READY COUNT" || headers[1] == "Ready Count" {
			// Layer 2
			widths[0] = 30
			widths[1] = 15
			widths[2] = 45
		} else if headers[1] == "COMPLETED COUNT" || headers[1] == "Completed Count" {
			// Layer 3
			widths[0] = 30
			widths[1] = 17
			widths[2] = 43
		} else if headers[0] == "METRIC" || headers[0] == "Metric" {
			// Layer 4: METRIC, VALUE, STATUS
			widths[0] = 30
			widths[1] = 20
			widths[2] = 40
		} else {
			widths[0] = 30
			widths[1] = 15
			widths[2] = 45
		}
	} else {
		// Fallback dynamic width calculation
		for i, h := range headers {
			widths[i] = len(h)
		}
		for _, row := range rows {
			for i, val := range row {
				if i < len(widths) {
					if len(val) > widths[i] {
						widths[i] = len(val)
					}
				}
			}
		}
	}

	return pkgcli.RenderTableWithTitleAndWrap(title, headers, widths, rows)
}

func renderTable(headers []string, rows [][]string) string {
	return renderTableWithTitle("", headers, rows)
}

func formatColumnHeader(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	if len(s) == 0 {
		return ""
	}
	return strings.ToUpper(s[0:1]) + s[1:]
}

func filterResultsByLayer(results []CheckResult, layer int) []CheckResult {
	var filtered []CheckResult
	for _, r := range results {
		status := strings.ToLower(r.Status)
		isTerminal := false
		if r.Status != "" && r.ObjectKind != "" {
			isTerminal, _ = objects.GetGlobalLifecycleLoader().IsTerminalStatusForKind(r.ObjectKind, r.Status)
		}
		hasIssues := len(r.Issues) > 0 || status == "error" || status == "failed"

		// Identify CAS/Integrity layer 0 issue
		hasCASIssue := false
		if len(r.Issues) > 0 {
			for _, issue := range r.Issues {
				if issue.Category == categoryIntegrity || issue.Category == categoryRegistration {
					hasCASIssue = true
					break
				}
			}
		}

		isPreliminary := false
		if r.Status != "" && r.ObjectKind != "" {
			isPreliminary, _ = objects.GetGlobalLifecycleLoader().IsPreliminaryStatusForKind(r.ObjectKind, r.Status)
		}

		switch layer {
		case 0:
			if hasCASIssue {
				filtered = append(filtered, r)
			}
		case 1:
			if (hasIssues && !hasCASIssue) || (!hasIssues && r.Status != "" && !isTerminal && isPreliminary) {
				filtered = append(filtered, r)
			}
		case 2:
			if !hasIssues && r.Status != "" && !isTerminal && !isPreliminary {
				filtered = append(filtered, r)
			}
		case 3:
			if !hasIssues && r.Status != "" && isTerminal {
				filtered = append(filtered, r)
			}
		}
	}
	return filtered
}

func normalizeErrorMessage(msg string) string {
	// 1. Normalize scheduler job IDs (SCH-run-bundle-XX)
	reSched := regexp.MustCompile(`SCH-run-bundle-\d+`)
	msg = reSched.ReplaceAllString(msg, "SCH-run-bundle-*")

	// 2. Normalize hex yaml filenames (64 hex characters + .yaml)
	reHexYaml := regexp.MustCompile(`[a-f0-9]{64}\.yaml`)
	msg = reHexYaml.ReplaceAllString(msg, "<hash>.yaml")

	// 3. Normalize general IDs in the message, e.g., "for object XXX" or "object XXX"
	// Only match strings starting with uppercase letters/digits or containing dashes/colons (avoiding lowercase words like get, exists, cache)
	reObjID := regexp.MustCompile(`object\s+([A-Z0-9][A-Za-z0-9_-]*:[A-Za-z0-9_-]+|[A-Z0-9]{2,10}-[A-Za-z0-9_-]+)`)
	msg = reObjID.ReplaceAllString(msg, "object <id>")

	// 4. Normalize file paths (anything containing .zqk/process/ or absolute paths)
	rePath := regexp.MustCompile(`/(?:Users|home)/[A-Za-z0-9_./-]+`)
	msg = rePath.ReplaceAllString(msg, "<path>")

	// 5. Normalize modification durations, e.g. "file modified 4m31s ago"
	reFileMod := regexp.MustCompile(`file modified\s+[0-9a-zA-Z.]+\s+ago`)
	msg = reFileMod.ReplaceAllString(msg, "file modified <duration> ago")

	// 6. Normalize any hex hashes with dots, e.g. "fc4c134c51989081..." or "fc4c134c51989081...."
	reHexDots := regexp.MustCompile(`(?i)[a-f0-9]{8,64}\.\.\.*`)
	msg = reHexDots.ReplaceAllString(msg, "<hash>...")

	return msg
}
