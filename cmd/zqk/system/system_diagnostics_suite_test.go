package system

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/agentonboard"
	"github.com/zqk-os/zqk/pkg/authcred"
	cli "github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/migration/scanner"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestSystemDiagnostic_NormalizeErrorMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normalize scheduler job ID",
			input:    "failed job SCH-run-bundle-12345 in worker",
			expected: "failed job SCH-run-bundle-* in worker",
		},
		{
			name:     "normalize hex yaml file",
			input:    "corrupt file 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef.yaml found",
			expected: "corrupt file <hash>.yaml found",
		},
		{
			name:     "normalize object ID in message",
			input:    "missing reference for object GOAL-1234-abcd",
			expected: "missing reference for object <id>",
		},
		{
			name:     "normalize user path",
			input:    "cannot write to /Users/john/repo/.zqk/process",
			expected: "cannot write to <path>",
		},
		{
			name:     "normalize home path",
			input:    "cannot write to /home/runner/work/zqk",
			expected: "cannot write to <path>",
		},
		{
			name:     "normalize file modified duration",
			input:    "stale cache file modified 4m31s ago",
			expected: "stale cache file modified <duration> ago",
		},
		{
			name:     "normalize hex dots",
			input:    "hash mismatch fc4c134c51989081... detected",
			expected: "hash mismatch <hash>... detected",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actual := normalizeErrorMessage(tc.input)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestSystemDiagnostic_FormatColumnHeader(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", formatColumnHeader(""))
	assert.Equal(t, "Backlog item", formatColumnHeader("backlog_item"))
	assert.Equal(t, "Goal", formatColumnHeader("goal"))
	assert.Equal(t, "Priority plan", formatColumnHeader("priority_plan"))
	assert.Equal(t, "Technical debt", formatColumnHeader("technical_debt"))
}

func TestSystemDiagnostic_FilterResultsByLayer(t *testing.T) {
	t.Parallel()

	results := []CheckResult{
		{
			ObjectID:   "GOAL-001",
			ObjectKind: "goal",
			Status:     "originated",
			Issues: []Issue{
				{Category: categoryIntegrity, Message: "hash mismatch", Tier: 1},
			},
		},
		{
			ObjectID:   "REQ-001",
			ObjectKind: "requirement",
			Status:     "originated",
			Issues: []Issue{
				{Category: "validation", Message: "title missing", Tier: 2},
			},
		},
		{
			ObjectID:   "BLI-001",
			ObjectKind: "backlog_item",
			Status:     "planned",
			Issues:     nil,
		},
		{
			ObjectID:   "BLI-002",
			ObjectKind: "backlog_item",
			Status:     "complete",
			Issues:     nil,
		},
	}

	layer0 := filterResultsByLayer(results, 0)
	assert.Len(t, layer0, 1)
	assert.Equal(t, "GOAL-001", layer0[0].ObjectID)

	layer1 := filterResultsByLayer(results, 1)
	assert.Len(t, layer1, 1)
	assert.Equal(t, "REQ-001", layer1[0].ObjectID)

	layer2 := filterResultsByLayer(results, 2)
	assert.Len(t, layer2, 1)
	assert.Equal(t, "BLI-001", layer2[0].ObjectID)

	layer3 := filterResultsByLayer(results, 3)
	assert.Len(t, layer3, 1)
	assert.Equal(t, "BLI-002", layer3[0].ObjectID)
}

func TestSystemDiagnostic_GroupAndFormatIssueRows(t *testing.T) {
	t.Parallel()

	results := []CheckResult{
		{
			ObjectID:   "BLI-001",
			ObjectKind: "backlog_item",
			Status:     "planned",
			Issues: []Issue{
				{Category: "lifecycle", Message: "missing goal_refs", Tier: 1, AutoFixable: true},
				{Category: "naming", Message: "title too short", Tier: 2, AutoFixable: false},
			},
		},
		{
			ObjectID:   "BLI-002",
			ObjectKind: "backlog_item",
			Status:     "planned",
			Issues: []Issue{
				{Category: "lifecycle", Message: "missing goal_refs", Tier: 1, AutoFixable: true},
				{Category: "info", Message: "consider adding persona", Tier: 3, AutoFixable: false},
				{Category: "hint", Message: "optional tag", Tier: 4, AutoFixable: false},
			},
		},
	}

	// Details = true
	rowsDetails := groupAndFormatIssueRows(results, true, nil, true)
	assert.NotEmpty(t, rowsDetails)

	// Details = false
	rowsNoDetails := groupAndFormatIssueRows(results, false, nil, true)
	assert.NotEmpty(t, rowsNoDetails)

	// With filter
	casFilter := func(i Issue) bool {
		return i.Category == categoryIntegrity
	}
	rowsFiltered := groupAndFormatIssueRows(results, false, casFilter, false)
	assert.Empty(t, rowsFiltered)

	// Check helper wrappers
	l0Rows := getGroupedLayer0Rows(results, false)
	assert.Empty(t, l0Rows) // No CAS issues in results

	issueRows := getGroupedIssueRows(results, true)
	assert.NotEmpty(t, issueRows)
}

func TestSystemDiagnostic_RenderTableWithTitle(t *testing.T) {
	t.Parallel()

	// Empty rows
	assert.Equal(t, "", renderTableWithTitle("Empty", []string{"Col1"}, nil))

	// 2 columns
	headers2 := []string{"Kind", "Count"}
	rows2 := [][]string{
		{"Goal", "5"},
		{"Requirement", "12"},
	}
	t2 := renderTableWithTitle("2-Col Table", headers2, rows2)
	assert.Contains(t, t2, "2-Col Table")
	assert.Contains(t, t2, "Goal")

	// 3 columns - Issue table
	headersIssue := []string{"Issue", "Severity", "Kind"}
	rowsIssue := [][]string{
		{"Missing goal reference", "❌", "Backlog item (2)"},
	}
	tIssue := renderTableWithTitle("Issues", headersIssue, rowsIssue)
	assert.Contains(t, tIssue, "Missing goal reference")

	// 3 columns - Ready Count
	headersReady := []string{"Kind", "Ready Count", "Status Breakdown"}
	rowsReady := [][]string{
		{"Goal", "3", "3 active"},
	}
	tReady := renderTableWithTitle("Ready", headersReady, rowsReady)
	assert.Contains(t, tReady, "READY COUNT")

	// 3 columns - Completed Count
	headersCompleted := []string{"Kind", "Completed Count", "Status Breakdown"}
	rowsCompleted := [][]string{
		{"Goal", "2", "2 complete"},
	}
	tCompleted := renderTableWithTitle("Completed", headersCompleted, rowsCompleted)
	assert.Contains(t, tCompleted, "COMPLETED COUNT")

	// 3 columns - Metric
	headersMetric := []string{"Metric", "Value", "Status"}
	rowsMetric := [][]string{
		{"Coverage", "85%", "Pass"},
	}
	tMetric := renderTableWithTitle("Metrics", headersMetric, rowsMetric)
	assert.Contains(t, tMetric, "Coverage")

	// 3 columns - Default
	headersDefault := []string{"A", "B", "C"}
	rowsDefault := [][]string{
		{"1", "2", "3"},
	}
	tDefault := renderTableWithTitle("Default 3-Col", headersDefault, rowsDefault)
	assert.Contains(t, tDefault, "Default 3-Col")

	// Fallback multi-column table
	headersMulti := []string{"C1", "C2", "C3", "C4"}
	rowsMulti := [][]string{
		{"v1", "v2", "v3", "v4"},
	}
	tMulti := renderTableWithTitle("Multi-Col", headersMulti, rowsMulti)
	assert.Contains(t, tMulti, "Multi-Col")

	// Wrapper renderTable
	tWrap := renderTable(headers2, rows2)
	assert.Contains(t, tWrap, "Goal")
}

func TestSystemDiagnostic_WriteLayeredSummary(t *testing.T) {
	cmd := NewCheckCmd()
	require.NotNil(t, cmd)

	results := []CheckResult{
		{
			ObjectID:   "GOAL-001",
			ObjectKind: "goal",
			Status:     "originated",
			Issues: []Issue{
				{Category: categoryIntegrity, Message: "CAS hash corrupt", Tier: 1},
			},
		},
		{
			ObjectID:   "REQ-001",
			ObjectKind: "requirement",
			Status:     "in_progress",
			Issues: []Issue{
				{Category: "validation", Message: "criteria_refs empty", Tier: 2},
			},
		},
		{
			ObjectID:   "BLI-001",
			ObjectKind: "backlog_item",
			Status:     "planned",
			Issues:     nil,
		},
		{
			ObjectID:   "BLI-002",
			ObjectKind: "backlog_item",
			Status:     "complete",
			Issues:     nil,
		},
		{
			ObjectID:   "OBJ-STATIC",
			ObjectKind: "spec",
			Status:     "",
			Issues:     nil,
		},
		{
			ObjectID:   "ERR-001",
			ObjectKind: "backlog_item",
			Status:     "error",
			Issues:     nil,
		},
	}

	var buf strings.Builder
	writeLayeredSummary(&buf, results, cmd)
	summary := buf.String()
	assert.Contains(t, summary, "System Check: High-Level Layered Summary")
	assert.Contains(t, summary, "Layer 0: CAS & Integrity Blockers")
	assert.Contains(t, summary, "Layer 1: Objects Needing Fixes")
	assert.Contains(t, summary, "Layer 2: Objects Ready for Execution/Promotion")
	assert.Contains(t, summary, "Layer 3: Completed and Validated Items")
	assert.Contains(t, summary, "Static configurations and specifications")

	// Empty results
	var emptyBuf strings.Builder
	writeLayeredSummary(&emptyBuf, nil, cmd)
	assert.Contains(t, emptyBuf.String(), "No CAS or integrity blockers found")
}

func TestSystemDiagnostic_GroupResultsByTier(t *testing.T) {
	t.Parallel()

	results := []CheckResult{
		{ObjectID: "T1", ObjectKind: "goal", Issues: []Issue{{Tier: 1, Message: "tier 1"}}},
		{ObjectID: "T2", ObjectKind: "goal", Issues: []Issue{{Tier: 2, Message: "tier 2"}}},
		{ObjectID: "T3", ObjectKind: "goal", Issues: []Issue{{Tier: 3, Message: "tier 3"}}},
		{ObjectID: "T4", ObjectKind: "goal", Issues: []Issue{{Tier: 4, Message: "tier 4"}}},
		{ObjectID: "CLEAN", ObjectKind: "goal", Issues: nil},
	}

	tier1, tier2, tier3, tier4, clean := groupResultsByTier(results)
	assert.Len(t, tier1, 1)
	assert.Len(t, tier2, 1)
	assert.Len(t, tier3, 1)
	assert.Len(t, tier4, 1)
	assert.Len(t, clean, 1)
}

func TestSystemDiagnostic_SeparatePublicAndInternal(t *testing.T) {
	t.Parallel()

	results := []CheckResult{
		{ObjectID: "GOAL-001", ObjectKind: "goal"},
		{ObjectID: "AE-001", ObjectKind: objects.KindAuditEvent},
		{ObjectID: "SPEC-001", ObjectKind: objects.KindObjectSpec},
	}

	pub, internal := separateByVisibility(results)
	assert.Len(t, pub, 1)
	assert.Equal(t, "GOAL-001", pub[0].ObjectID)
	assert.Len(t, internal, 2)

	grouped := &TierGroupedResults{
		Tier1: []CheckResult{{ObjectID: "GOAL-001", ObjectKind: "goal"}},
		Clean: []CheckResult{{ObjectID: "SPEC-001", ObjectKind: objects.KindObjectSpec}},
	}
	separatePublicAndInternal(grouped)
	assert.Len(t, grouped.PublicTier1, 1)
	assert.Len(t, grouped.InternalClean, 1)
}

func TestSystemDiagnostic_WritePublicAndInternalSummary(t *testing.T) {
	t.Parallel()

	grouped := &TierGroupedResults{
		PublicTier1:   []CheckResult{{ObjectID: "P1", ObjectKind: "goal"}},
		PublicTier2:   []CheckResult{{ObjectID: "P2", ObjectKind: "goal"}},
		PublicTier3:   []CheckResult{{ObjectID: "P3", ObjectKind: "goal"}},
		PublicTier4:   []CheckResult{{ObjectID: "P4", ObjectKind: "goal"}},
		PublicClean:   []CheckResult{{ObjectID: "PC", ObjectKind: "goal"}},
		InternalTier1: []CheckResult{{ObjectID: "I1", ObjectKind: objects.KindObjectSpec}},
		InternalTier2: []CheckResult{{ObjectID: "I2", ObjectKind: objects.KindObjectSpec}},
		InternalTier3: []CheckResult{{ObjectID: "I3", ObjectKind: objects.KindObjectSpec}},
		InternalTier4: []CheckResult{{ObjectID: "I4", ObjectKind: objects.KindObjectSpec}},
		InternalClean: []CheckResult{{ObjectID: "IC", ObjectKind: objects.KindObjectSpec}},
	}

	// Inactive tier filter
	var pubBuf strings.Builder
	writePublicSummary(&pubBuf, grouped, false, 0)
	assert.Contains(t, pubBuf.String(), "Public Objects:")
	assert.Contains(t, pubBuf.String(), "Tier 1 (Blocking): 1")
	assert.Contains(t, pubBuf.String(), "Clean: 1")

	// Active tier filters 1..4
	for tier := 1; tier <= 4; tier++ {
		var b strings.Builder
		writePublicSummary(&b, grouped, true, tier)
		assert.Contains(t, b.String(), "Public Objects:")
	}

	// Internal summary - inactive filter
	var intBuf strings.Builder
	writeInternalSummary(&intBuf, grouped, false, 0)
	assert.Contains(t, intBuf.String(), "System Health (Internal Objects):")
	assert.Contains(t, intBuf.String(), "Clean: 1")

	// Internal summary - active filter 1..4
	for tier := 1; tier <= 4; tier++ {
		var b strings.Builder
		writeInternalSummary(&b, grouped, true, tier)
		assert.Contains(t, b.String(), "System Health (Internal Objects):")
	}
}

func TestSystemDiagnostic_WriteTierDetails(t *testing.T) {
	t.Parallel()

	grouped := &TierGroupedResults{
		PublicTier1: []CheckResult{
			{
				ObjectID:   "PUB-1",
				ObjectKind: "goal",
				Issues: []Issue{
					{Category: categoryIntegrity, Message: "hash corrupt", Tier: 1, FixCommand: "system fix"},
				},
			},
		},
		InternalTier1: []CheckResult{
			{
				ObjectID:   "INT-1",
				ObjectKind: objects.KindObjectSpec,
				Issues: []Issue{
					{Category: categoryRegistration, Message: "spec unregistered", Tier: 1, FixCommand: "spec register"},
				},
			},
		},
		PublicTier2: []CheckResult{
			{
				ObjectID:   "PUB-2",
				ObjectKind: "goal",
				Issues: []Issue{
					{Category: "validation", Message: "Field title is required", Tier: 2},
				},
			},
		},
		InternalTier2: []CheckResult{
			{
				ObjectID:   "INT-2",
				ObjectKind: objects.KindObjectSpec,
				Issues: []Issue{
					{Category: "validation", Message: "Field id has invalid value", Tier: 2},
				},
			},
		},
		PublicTier3: []CheckResult{
			{
				ObjectID:   "PUB-3",
				ObjectKind: "goal",
				Issues: []Issue{
					{Category: "hint", Message: "description (Tier 3): recommended", Tier: 3},
				},
			},
		},
		InternalTier3: []CheckResult{
			{
				ObjectID:   "INT-3",
				ObjectKind: objects.KindObjectSpec,
				Issues: []Issue{
					{Category: "hint", Message: "version (Tier 3): optional", Tier: 3},
				},
			},
		},
	}

	cliCtx := cli.ContextForProjectRoot(t.TempDir()).WithVerbose(true)

	var t1Buf strings.Builder
	writeTier1Details(&t1Buf, grouped, 80, cliCtx)
	assert.Contains(t, t1Buf.String(), "Tier 1: Blocking Issues")
	assert.Contains(t, t1Buf.String(), "Fix: system fix")

	var t2Buf strings.Builder
	writeTier2Details(&t2Buf, grouped, 80)
	assert.Contains(t, t2Buf.String(), "Tier 2: Warnings (Public Objects)")
	assert.Contains(t, t2Buf.String(), "Tier 2: Warnings (Internal Objects)")

	var t3Buf strings.Builder
	writeTier3Details(&t3Buf, grouped, 80, cliCtx)
	assert.Contains(t, t3Buf.String(), "Tier 3: Informational (Public Objects)")
	assert.Contains(t, t3Buf.String(), "Tier 3: Informational (Internal Objects)")

	// Non-verbose internal details
	cliCtxQuiet := cli.ContextForProjectRoot(t.TempDir()).WithVerbose(false)
	var t1QuietBuf strings.Builder
	writeInternalTier1Details(&t1QuietBuf, grouped.InternalTier1, 80, cliCtxQuiet)
	assert.Contains(t, t1QuietBuf.String(), "System Health (Internal Objects)")
}

func TestSystemDiagnostic_ExtractFieldName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "title", extractFieldName("Field title is required"))
	assert.Equal(t, "status", extractFieldName("Field status: has invalid value"))
	assert.Equal(t, "description", extractFieldName("description (Tier 1): invalid"))
	assert.Equal(t, "", extractFieldName("General error occurred"))
}

func TestSystemDiagnostic_WriteStatisticsAndResolution(t *testing.T) {
	t.Parallel()

	results := []CheckResult{
		{
			ObjectID:   "GOAL-001",
			ObjectKind: "goal",
			AutoFixed:  []string{"normalized title", "set namespace"},
		},
	}
	grouped := &TierGroupedResults{
		PublicTier1: []CheckResult{results[0]},
		Clean:       nil,
	}

	var statsBuf strings.Builder
	writeDetailedStatistics(&statsBuf, results, grouped)
	assert.Contains(t, statsBuf.String(), "Detailed Statistics")
	assert.Contains(t, statsBuf.String(), "Total objects checked: 1")

	var autoFixBuf strings.Builder
	writeAutoFixedItems(&autoFixBuf, results, 80)
	assert.Contains(t, autoFixBuf.String(), "Auto-Fixed Issues")
	assert.Contains(t, autoFixBuf.String(), "normalized title")

	var resBuf strings.Builder
	staleCAS := &StaleCASCleanupResult{
		KindsRun:     []string{"goal"},
		FilesHandled: 2,
	}
	writeResolutionSummary(&resBuf, results, staleCAS, 80)
	assert.Contains(t, resBuf.String(), "Resolution")
	assert.Contains(t, resBuf.String(), "Auto-fixed: 2 issue(s)")
	assert.Contains(t, resBuf.String(), "Stale CAS cleanup: ran for kind(s) goal")

	var auditBuf strings.Builder
	writeAuditEventBufferStatus(&auditBuf, 3, map[string]int{"mutation": 2, "read": 1})
	assert.Contains(t, auditBuf.String(), "Audit Events")
	assert.Contains(t, auditBuf.String(), "3 audit event(s) are being buffered")
	assert.Contains(t, auditBuf.String(), "mutation: 2 occurrence(s)")
}

func TestSystemDiagnostic_OutputResultsHelpers(t *testing.T) {
	t.Parallel()

	cmd := NewCheckCmd()
	if err := cmd.Flags().Set("format", "table"); err != nil {
		t.Logf("set format err: %v", err)
	}
	if err := cmd.Flags().Set("verbose", "true"); err != nil {
		t.Logf("set verbose err: %v", err)
	}

	cliCtx := cli.ContextForProjectRoot(t.TempDir()).WithVerbose(true)
	outCtx, err := initializeOutputResultsContext(cmd, cliCtx, nil)
	assert.NoError(t, err)
	assert.NotNil(t, outCtx)
	assert.Equal(t, "table", outCtx.FormatStr)

	// Command string building
	cmdStr := buildCommandString(cmd)
	assert.Contains(t, cmdStr, "system check")
	assert.Contains(t, cmdStr, "--verbose")

	// Calculate sync interval
	assert.Equal(t, 1, calculateSyncInterval(50))
	assert.Equal(t, 100, calculateSyncInterval(5000))

	// Slice conversion
	checkResults := []CheckResult{{ObjectID: "A"}, {ObjectID: "B"}}
	slice := convertResultsToInterfaceSlice(checkResults)
	assert.Len(t, slice, 2)

	// Setup output writer
	w, f, err := setupOutputWriter(cmd, "")
	assert.NoError(t, err)
	assert.Nil(t, f)
	assert.NotNil(t, w)
}

func TestSystemDiagnostic_AlignHelpers(t *testing.T) {
	t.Parallel()

	// GoalRefsFromObject
	obj := map[string]any{
		objects.FieldKeyGoalRefs: []any{"GOAL-001", "GOAL-002"},
	}
	refs := GoalRefsFromObject(obj)
	assert.True(t, refs["GOAL-001"])
	assert.True(t, refs["GOAL-002"])

	// RoundTwo
	assert.Equal(t, 75.56, RoundTwo(75.555))
	assert.Equal(t, 100.0, RoundTwo(100.0))
	assert.Equal(t, 0.0, RoundTwo(0.0))

	// TruncateGaps
	items := []map[string]any{{"id": "1"}, {"id": "2"}, {"id": "3"}}
	assert.Len(t, TruncateGaps(items, 2), 2)
	assert.Len(t, TruncateGaps(items, 5), 3)

	// OutputAlignDashboard - table & json
	cmd := NewAlignCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))

	err := OutputAlignDashboard(cmd, map[string]any{}, 85.0, 10, 8, 2, 3)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "SYSTEM ALIGNMENT DASHBOARD")

	jsonBuf := new(bytes.Buffer)
	jsonCmd := NewAlignCmd()
	jsonCmd.SetOut(jsonBuf)
	jsonCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), jsonBuf))
	if err := jsonCmd.Flags().Set("format", "json"); err != nil {
		t.Logf("set format err: %v", err)
	}
	err = OutputAlignDashboard(jsonCmd, map[string]any{}, 85.0, 10, 8, 2, 3)
	assert.NoError(t, err)
	assert.Contains(t, jsonBuf.String(), "overall_alignment_score")

	// OutputAlignTable
	tblBuf := new(bytes.Buffer)
	tblCmd := NewAlignCmd()
	tblCmd.SetOut(tblBuf)
	tblCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), tblBuf))
	result := map[string]any{
		"alignment": map[string]any{
			"goal_work": map[string]any{
				"total_work_items":    10,
				"items_with_goals":    7,
				"items_without_goals": 3,
				"goal_coverage_pct":   70.0,
			},
		},
		"alignment_score": 70.0,
		"gaps": []map[string]any{
			{objects.FieldKeyID: "BLI-GAP-001", objects.FieldKeyTitle: "Unguided task"},
		},
	}
	err = OutputAlignTable(tblCmd, result, true, "GOAL-001")
	assert.NoError(t, err)
	assert.Contains(t, tblBuf.String(), "Strategic alignment (goal-work)")
	assert.Contains(t, tblBuf.String(), "Goal filter:           GOAL-001")
	assert.Contains(t, tblBuf.String(), "BLI-GAP-001")
}

func TestSystemDiagnostic_WhoamiHelpers(t *testing.T) {
	t.Parallel()

	// parseMCPRoles & parseMCPPermissions
	assert.Nil(t, parseMCPRoles(""))
	roles := parseMCPRoles("admin, engineer, auditor")
	assert.Equal(t, []string{"admin", "engineer", "auditor"}, roles)

	assert.Nil(t, parseMCPPermissions(""))
	perms := parseMCPPermissions("read, write, delete")
	assert.Equal(t, []string{"read", "write", "delete"}, perms)

	// WhoamiAccountInfo & buildWhoamiOutputData
	info := &WhoamiAccountInfo{
		AccountID:   "ACC-DEV-001",
		Roles:       roles,
		Permissions: perms,
		PersonaID:   "PER-TEST",
		Lane:        "doer",
		AccountDetails: map[string]any{
			objects.FieldKeyDisplayName: "Test Developer",
			objects.FieldKeyEmail:       "test@example.com",
			objects.FieldKeyUsername:    "testdev",
			objects.FieldKeyTitle:       "Software Craftsman",
		},
	}

	outData := buildWhoamiOutputData(info)
	assert.Equal(t, "ACC-DEV-001", outData[objects.FieldKeyAccountID])
	assert.Equal(t, "Test Developer", outData[objects.FieldKeyDisplayName])
	assert.Equal(t, "doer", outData["lane"])

	// formatWhoamiTable helpers
	assert.Contains(t, formatWhoamiTableAccount(outData), "ACC-DEV-001")
	assert.Contains(t, formatWhoamiTableLane(outData), "doer")
	assert.Contains(t, formatWhoamiTablePersona(outData), "PER-TEST")
	assert.Contains(t, formatWhoamiTableName(outData), "Test Developer")
	assert.Contains(t, formatWhoamiTableUserInfo(outData), "testdev")
	assert.Contains(t, formatWhoamiTableRoles(outData), "admin, engineer, auditor")
	assert.Contains(t, formatWhoamiTablePermissions(outData), "read, write, delete")
	assert.Contains(t, formatWhoamiTableContext(outData), "CLI")

	// outputWhoamiTable
	cmd := NewWhoamiCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))
	err := outputWhoamiTable(cmd, outData)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Current User Account")
	assert.Contains(t, buf.String(), "ACC-DEV-001")
}

func TestSystemDiagnostic_StatusHelpers(t *testing.T) {
	t.Parallel()

	// extractProjectName
	yamlContent := "project:\n  name: \"my-awesome-project\"\nversion: 1\n"
	assert.Equal(t, "my-awesome-project", extractProjectName(yamlContent))
	assert.Equal(t, "", extractProjectName("invalid yaml"))

	// determineProjectStatus
	tmpDir := t.TempDir()
	status, msg := determineProjectStatus(tmpDir)
	assert.Equal(t, projectStatusIncomplete, status)
	assert.Contains(t, msg, "directory not found")

	dotProcess := filepath.Join(tmpDir, paths.ProcessDir)
	if err := fileutil.MkdirAll(dotProcess, paths.DirPerm755); err != nil {
		t.Logf("mkdir err: %v", err)
	}
	statusInit, msgInit := determineProjectStatus(tmpDir)
	assert.Equal(t, projectStatusInitialized, statusInit)
	assert.Equal(t, "Initialized", msgInit)

	// formatStatusTable helpers
	statusData := map[string]any{
		"project":              "Test Project",
		"root":                 tmpDir,
		objects.FieldKeyStatus: projectStatusInitialized,
		"system_health": map[string]any{
			objects.FieldKeyStatus: "healthy",
			"scheduler": map[string]any{
				"running": true,
			},
		},
		"current_priority_plan": map[string]any{
			objects.FieldKeyID:    "PRI-001",
			objects.FieldKeyTitle: "Cycle 45 Execution",
		},
		"storage": map[string]any{
			"backend": "file-based",
		},
		"recent_activity": []string{"Activity 1", "Activity 2"},
	}

	assert.Contains(t, formatStatusTableHeader(), "ZQK Project Status")
	assert.Contains(t, formatStatusTableProject(statusData), "Test Project")
	assert.Contains(t, formatStatusTableStatus(statusData), "Initialized (Healthy)")
	assert.Contains(t, formatStatusTablePlan(statusData), "PRI-001")
	assert.Contains(t, formatStatusTableHealth(statusData), "healthy")
	assert.Contains(t, formatStatusTableStorage(statusData), "file-based")
	assert.Contains(t, formatStatusTableActivity(statusData), "Activity 1")

	// outputStatusTable
	cmd := NewStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))
	err := outputStatusTable(cmd, statusData)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "ZQK Project Status")

	// identityStatusMap
	snap := authcred.IdentityStatus{
		AccountID: "ACC-001",
		Username:  "tester",
		Title:     "Engineer",
		PersonaID: "PER-ENG",
		Roles:     []string{"admin"},
		Lane:      "admin",
	}
	m := identityStatusMap(snap)
	assert.Equal(t, "ACC-001", m[objects.FieldKeyAccountID])
	assert.Equal(t, "admin", m["lane"])
}

func TestSystemDiagnostic_AgentLockdown(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() {
		testkit.ScrubProjectRootForTempCleanup(tmpDir, 40, 25*time.Millisecond)
	})

	t.Setenv(zqkenv.ProjectRoot().Name(), tmpDir)
	zqkenv.ProjectRoot().Set(tmpDir)

	for _, d := range []string{".gemini", ".ide", ".claude"} {
		target := filepath.Join(tmpDir, d)
		if err := fileutil.MkdirAll(target, paths.DirPerm755); err != nil {
			t.Logf("mkdir err: %v", err)
		}
	}

	cmd := NewAgentLockdownCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "agent-lockdown", cmd.Use)

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), buf)
	cmd.SetContext(ctx)

	// Run lock
	err := runAgentLockdown(cmd, false)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "locked down")

	// Run unlock
	buf.Reset()
	err = runAgentLockdown(cmd, true)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "unlocked")
}

func TestSystemDiagnostic_CheckPolicy(t *testing.T) {
	cmd := NewCheckPolicyCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "check-policy", cmd.Use)
	assert.Contains(t, cmd.Aliases, "policy-check")

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), buf)
	cmd.SetContext(ctx)

	// Execute with --help or a safe dry run
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "check-policy")
}

func TestSystemDiagnostic_CheckFastContract(t *testing.T) {
	t.Parallel()

	// nil cmd
	assert.NoError(t, refuseReducedSurfaceWithMutatingFlags(nil))
	assert.NoError(t, refuseFastWithMutatingFlags(nil))
	assert.False(t, checkRefsReduced(nil))
	assert.False(t, checkFastModeEnabled(nil))

	cmd := NewCheckCmd()
	assert.False(t, checkRefsReduced(cmd))
	assert.False(t, checkFastModeEnabled(cmd))

	// Set fast
	if err := cmd.Flags().Set(checkFlagFast, "true"); err != nil {
		t.Logf("set flag err: %v", err)
	}
	assert.True(t, checkRefsReduced(cmd))
	assert.True(t, checkFastModeEnabled(cmd))

	// Refuse with auto-fix
	if err := cmd.Flags().Set(checkFlagAutoFix, "true"); err != nil {
		t.Logf("set flag err: %v", err)
	}
	err := refuseReducedSurfaceWithMutatingFlags(cmd)
	assert.NoError(t, err)
	// Fast was reset to false
	assert.False(t, checkRefsReduced(cmd))

	passLine := checkFastPartialPassLine()
	assert.Contains(t, passLine, checkFastPartialVerdict)
}

func TestSystemDiagnostic_CheckGuidance(t *testing.T) {
	t.Parallel()

	// Default full check timeout
	cmd := NewCheckCmd()
	dur := CalculateCheckProgressiveTimeout(cmd, nil)
	assert.Equal(t, defaultFullCheckTimeout, dur)

	// Targeted object check
	durTargeted := CalculateCheckProgressiveTimeout(cmd, []string{"BLI-123"})
	assert.Equal(t, defaultTargetedCheckTimeout, durTargeted)

	// Kind check
	durKind := CalculateCheckProgressiveTimeout(cmd, []string{"backlog_item"})
	assert.Equal(t, defaultKindCheckTimeout, durKind)

	// Fast check
	fastCmd := NewCheckCmd()
	if err := fastCmd.Flags().Set("fast", "true"); err != nil {
		t.Logf("set flag err: %v", err)
	}
	assert.Equal(t, defaultFastCheckTimeout, CalculateCheckProgressiveTimeout(fastCmd, nil))

	// Auto-fix check
	fixCmd := NewCheckCmd()
	if err := fixCmd.Flags().Set("auto-fix", "true"); err != nil {
		t.Logf("set flag err: %v", err)
	}
	assert.Equal(t, defaultAutofixCheckTimeout, CalculateCheckProgressiveTimeout(fixCmd, nil))

	// Timeout error format
	timeoutErr := FormatCheckTimeoutError(cmd, []string{"all"}, 30*time.Minute)
	assert.Error(t, timeoutErr)
	assert.Contains(t, timeoutErr.Error(), "system check timed out")
	assert.Contains(t, timeoutErr.Error(), "Progressive Timeout")
}

func TestSystemDiagnostic_CheckKindHelpers(t *testing.T) {
	t.Parallel()

	// shouldProcessFileForCheck
	file := &scanner.YAMLFile{
		ObjectID: "GOAL-001",
		Path:     "/path/to/GOAL-001.yaml",
	}

	assert.True(t, shouldProcessFileForCheck(file, nil, nil))
	assert.True(t, shouldProcessFileForCheck(file, []string{"GOAL-001"}, nil))
	assert.False(t, shouldProcessFileForCheck(file, []string{"GOAL-999"}, nil))

	// determineActualKind
	checkCtx := &CheckKindContext{Kind: "goal"}
	obj := &parser.ParsedObject{ID: "GOAL-001", Kind: "goal"}
	assert.Equal(t, "goal", determineActualKind(checkCtx, obj))

	objNoKind := &parser.ParsedObject{ID: "BLI-001"}
	assert.Equal(t, objects.KindBacklogItem, determineActualKind(checkCtx, objNoKind))

	// createDeferredChecksPtr
	cmd := NewCheckCmd()
	checkCtx.Cmd = cmd
	assert.Nil(t, createDeferredChecksPtr(checkCtx))

	if err := cmd.Flags().Set("auto-fix", "true"); err != nil {
		t.Logf("set flag err: %v", err)
	}
	assert.NotNil(t, createDeferredChecksPtr(checkCtx))
}

func TestSystemDiagnostic_CheckNotify(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	results := []CheckResult{
		{
			ObjectID: "E-1",
			Status:   objects.ObjectStatusError,
			Issues: []Issue{
				{Tier: 1, Message: "tier 1"},
				{Tier: 2, Message: "tier 2"},
				{Tier: 3, Message: "tier 3"},
				{Tier: 4, Message: "tier 4"},
			},
		},
	}

	sum := buildSystemCheckWakeSummary(tmpDir, results)
	assert.Equal(t, 1, sum.ErrorStatusObjects)
	assert.Equal(t, 1, sum.BlockingIssues)
	assert.Equal(t, 1, sum.Warnings)
	assert.Equal(t, 1, sum.Informational)
	assert.Equal(t, 1, sum.Recommendations)

	// maybeNotifySystemCheckWake nil or unchanged
	maybeNotifySystemCheckWake(nil, tmpDir, results)
	cmd := NewCheckCmd()
	maybeNotifySystemCheckWake(cmd, tmpDir, results)
}

func TestSystemDiagnostic_Reminders(t *testing.T) {
	t.Parallel()

	// formatReminderDuration
	assert.Equal(t, "45s", formatReminderDuration(45*time.Second))
	assert.Equal(t, "20m", formatReminderDuration(20*time.Minute))
	assert.Equal(t, "2.5h", formatReminderDuration(150*time.Minute))

	// checkCLIReminder with missing flag file
	tmpDir := t.TempDir()
	cmd := NewCheckCmd()
	assert.False(t, checkCLIReminder(nil, tmpDir))
	assert.False(t, checkCLIReminder(cmd, ""))
	assert.False(t, checkCLIReminder(cmd, tmpDir))

	// with active flag file
	flagDir := filepath.Join(tmpDir, paths.ProjectDataDir)
	if err := fileutil.MkdirAll(flagDir, paths.DirPerm755); err != nil {
		t.Logf("mkdir err: %v", err)
	}
	flagFile := filepath.Join(flagDir, "cli_reminder.flag")
	if err := fileutil.WriteFile(flagFile, []byte("Test reminder message"), paths.FilePerm644); err != nil {
		t.Logf("write err: %v", err)
	}

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))
	assert.True(t, checkCLIReminder(cmd, tmpDir))
	assert.Contains(t, buf.String(), "CLI-FIRST REMINDER")

	// NewRemindersCmd
	remCmd := NewRemindersCmd()
	require.NotNil(t, remCmd)
	assert.Equal(t, "reminders", remCmd.Use)
}

func TestSystemDiagnostic_AggregateAudit(t *testing.T) {
	t.Parallel()

	cmd := NewAggregateAuditCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "aggregate-audit", cmd.Use)

	// parseDuration
	d24, err := parseDuration("24h")
	assert.NoError(t, err)
	assert.Equal(t, 24*time.Hour, d24)

	d7d, err := parseDuration("7d")
	assert.NoError(t, err)
	assert.Equal(t, 7*24*time.Hour, d7d)

	_, err = parseDuration("invalid")
	assert.Error(t, err)

	// parseTimeWindowWithNow
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	start, end, err := parseTimeWindowWithNow(cmd, now)
	assert.NoError(t, err)
	assert.Equal(t, now, end)
	assert.Equal(t, now.Add(-24*time.Hour), start)

	// explicit start/end
	if err := cmd.Flags().Set("start", "2026-11-01T00:00:00Z"); err != nil {
		t.Logf("set start err: %v", err)
	}
	if err := cmd.Flags().Set("end", "2026-11-02T00:00:00Z"); err != nil {
		t.Logf("set end err: %v", err)
	}
	startExp, endExp, err := parseTimeWindowWithNow(cmd, now)
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), startExp)
	assert.Equal(t, time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC), endExp)
}

func TestSystemDiagnostic_AgentOnboardSummary(t *testing.T) {
	cmd := NewAgentOnboardCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "agent-onboard", cmd.Use)

	res := &agentonboard.Result{
		Status: agentonboard.ResultSuccess,
		Stages: map[string]agentonboard.StageResult{
			agentonboard.StageDetect: {
				Status: agentonboard.ResultSuccess,
				Detail: map[string]any{
					"detected": []agentonboard.DetectedVendor{
						{DisplayName: "Cursor", Markers: []string{".cursor"}},
					},
				},
			},
			agentonboard.StagePrimeWorkspace: {
				Status: agentonboard.ResultSuccess,
				Detail: map[string]any{
					"files": []string{".agents/AGENTS.md"},
				},
			},
			agentonboard.StageSeat: {
				Status: agentonboard.ResultSuccess,
				Detail: map[string]any{
					"created": 3,
				},
			},
		},
	}

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))

	renderAgentOnboardSummary(cmd, res)
	output := buf.String()
	assert.Contains(t, output, "Agent Onboard & Environment Sync")
	assert.Contains(t, output, "Detected Agent Hosts:")
	assert.Contains(t, output, "Cursor")
	assert.Contains(t, output, "Seeded 3 default personas")
}

func TestSystemDiagnostic_QuickstartAndStartHere(t *testing.T) {
	t.Parallel()

	guideInit := renderQuickstartGuide(t.TempDir())
	assert.Contains(t, guideInit, "Quickstart")

	guideUninit := renderQuickstartGuide("")
	assert.Contains(t, guideUninit, "Quickstart")

	startHereCmd := NewStartHereCmd()
	require.NotNil(t, startHereCmd)
	assert.Equal(t, "start-here", startHereCmd.Use)

	quickCmd := NewQuickstartCmd()
	require.NotNil(t, quickCmd)
	assert.Equal(t, "quickstart", quickCmd.Use)
}

func TestSystemDiagnostic_OtherCommandsConstruction(t *testing.T) {
	t.Parallel()

	hygieneCmd := NewResourceHygieneCmd()
	require.NotNil(t, hygieneCmd)
	assert.Equal(t, "resource-hygiene", hygieneCmd.Use)

	branchesCmd := NewCleanBranchesCmd()
	require.NotNil(t, branchesCmd)
	assert.Equal(t, "clean-branches", branchesCmd.Use)

	walCmd := NewCompactWALCmd()
	require.NotNil(t, walCmd)
	assert.Equal(t, "compact-wal", walCmd.Use)

	journalCmd := NewCompactJournalCmd()
	require.NotNil(t, journalCmd)
	assert.Equal(t, "compact-journal", journalCmd.Use)

	testDriveCmd := NewTestDriveCmd()
	require.NotNil(t, testDriveCmd)
	assert.Equal(t, "test-drive", testDriveCmd.Use)

	valAgentRulesCmd := NewValidateAgentRulesCmd()
	require.NotNil(t, valAgentRulesCmd)
	assert.Equal(t, "validate-agent-rules", valAgentRulesCmd.Use)

	valCmdSpecsCmd := NewValidateCommandSpecsCmd()
	require.NotNil(t, valCmdSpecsCmd)
	assert.Equal(t, "validate-command-specs", valCmdSpecsCmd.Use)

	objCountCmd := NewObjectCountReportCmd()
	require.NotNil(t, objCountCmd)
	assert.Equal(t, "object-count-report", objCountCmd.Use)

	objHygieneCmd := NewObjectHygieneScanCmd()
	require.NotNil(t, objHygieneCmd)
	assert.Equal(t, "object-hygiene-scan", objHygieneCmd.Use)

	promptCmd := NewPromptBuilderCmd()
	require.NotNil(t, promptCmd)
	assert.Contains(t, promptCmd.Use, "prompt-builder")
}
