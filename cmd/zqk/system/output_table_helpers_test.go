package system

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cli "github.com/zqk-os/zqk/pkg/cliapp"
)

func TestOutputTableHelpers_GroupAndSeparate(t *testing.T) {
	t.Parallel()

	resTier1 := CheckResult{
		ObjectID:   "GOAL-001",
		ObjectKind: "goal",
		Issues: []Issue{
			{Tier: 1, Message: "Missing title", Category: "schema"},
		},
	}
	resTier2 := CheckResult{
		ObjectID:   "BLI-001",
		ObjectKind: "backlog_item",
		Issues: []Issue{
			{Tier: 2, Message: "Missing requirement_refs in field reqs", Category: "graph"},
		},
	}
	resTier3 := CheckResult{
		ObjectID:   "PER-001",
		ObjectKind: "persona",
		Issues: []Issue{
			{Tier: 3, Message: "Deprecated warning in field notes", Category: "deprecation"},
		},
	}
	resTier4 := CheckResult{
		ObjectID:   "TST-001",
		ObjectKind: "test_case",
		Issues: []Issue{
			{Tier: 4, Message: "Info advisory", Category: "advisory"},
		},
	}
	resClean := CheckResult{
		ObjectID:   "CRIT-001",
		ObjectKind: "criteria",
		Issues:     nil,
	}
	resInternal := CheckResult{
		ObjectID:   "SPEC-001",
		ObjectKind: "object_spec",
		FilePath:   ".zqk/specs/_internal/spec.yaml",
		Issues: []Issue{
			{Tier: 1, Message: "Internal tier 1 issue", Category: "spec"},
		},
	}

	all := []CheckResult{resTier1, resTier2, resTier3, resTier4, resClean, resInternal}

	t1, t2, t3, t4, clean := groupResultsByTier(all)
	assert.Len(t, t1, 2) // resTier1 + resInternal
	assert.Len(t, t2, 1)
	assert.Len(t, t3, 1)
	assert.Len(t, t4, 1)
	assert.Len(t, clean, 1)

	grouped := &TierGroupedResults{
		Tier1: t1,
		Tier2: t2,
		Tier3: t3,
		Tier4: t4,
		Clean: clean,
	}

	separatePublicAndInternal(grouped)
	assert.Len(t, grouped.PublicTier1, 1)
	assert.Len(t, grouped.InternalTier1, 1)

	// Test getTerminalWidth fallback
	width := getTerminalWidth()
	assert.GreaterOrEqual(t, width, 40)
}

func TestOutputTableHelpers_WriteSummariesAndDetails(t *testing.T) {
	t.Parallel()

	res1 := CheckResult{
		ObjectID:   "REQ-001",
		ObjectKind: "requirement",
		FilePath:   "requirements/REQ-001.yaml",
		Issues: []Issue{
			{Tier: 1, Message: "Required field missing: 'title'", Category: categoryIntegrity},
		},
	}
	res2 := CheckResult{
		ObjectID:   "CRIT-002",
		ObjectKind: "criteria",
		FilePath:   "criteria/CRIT-002.yaml",
		Issues: []Issue{
			{Tier: 2, Message: "Missing parent ref"},
		},
	}
	res3 := CheckResult{
		ObjectID:   "BLI-003",
		ObjectKind: "backlog_item",
		FilePath:   "backlog/BLI-003.yaml",
		Issues: []Issue{
			{Tier: 3, Message: "Advisory note"},
		},
	}
	resAuto := CheckResult{
		ObjectID:   "BLI-004",
		ObjectKind: "backlog_item",
		FilePath:   "backlog/BLI-004.yaml",
		AutoFixed:  []string{"Updated status to in_progress"},
	}

	grouped := &TierGroupedResults{
		Tier1:         []CheckResult{res1},
		Tier2:         []CheckResult{res2},
		Tier3:         []CheckResult{res3},
		PublicTier1:   []CheckResult{res1},
		PublicTier2:   []CheckResult{res2},
		PublicTier3:   []CheckResult{res3},
		InternalTier1: []CheckResult{res1},
		InternalTier2: []CheckResult{res2},
		InternalTier3: []CheckResult{res3},
	}

	var buf strings.Builder
	writePublicSummary(&buf, grouped, false, 0)
	assert.Contains(t, buf.String(), "Public Objects")

	buf.Reset()
	writePublicSummary(&buf, grouped, true, 1)
	assert.Contains(t, buf.String(), "Public Objects")

	buf.Reset()
	writeInternalSummary(&buf, grouped, false, 0)
	assert.Contains(t, buf.String(), "System Health (Internal Objects)")

	buf.Reset()
	writeInternalSummary(&buf, grouped, true, 2)
	assert.Contains(t, buf.String(), "System Health (Internal Objects)")

	cliCtx := cli.ContextForProjectRoot("")
	cliCtx.Verbose = true
	buf.Reset()
	writeTier1Details(&buf, grouped, 80, cliCtx)
	assert.Contains(t, buf.String(), "REQ-001")

	buf.Reset()
	writeInternalTier1Details(&buf, []CheckResult{res1}, 80, cliCtx)
	assert.Contains(t, buf.String(), "REQ-001")

	buf.Reset()
	writeTier2Details(&buf, grouped, 80)
	assert.Contains(t, buf.String(), "CRIT-002")

	buf.Reset()
	writeTier3Details(&buf, grouped, 80, cliCtx)
	assert.Contains(t, buf.String(), "BLI-003")

	buf.Reset()
	writeInternalTier3Details(&buf, []CheckResult{res3}, 80, cliCtx)
	assert.Contains(t, buf.String(), "BLI-003")

	buf.Reset()
	writeDetailedStatistics(&buf, []CheckResult{res1, res2, res3}, grouped)
	assert.Contains(t, buf.String(), "Statistics")

	buf.Reset()
	writeAutoFixedItems(&buf, []CheckResult{resAuto}, 80)
	assert.Contains(t, buf.String(), "Auto-Fixed")

	buf.Reset()
	writeResolutionSummary(&buf, []CheckResult{res1}, &StaleCASCleanupResult{FilesHandled: 2}, 80)
	assert.Contains(t, buf.String(), "Resolution")

	buf.Reset()
	writeAuditEventBufferStatus(&buf, 3, map[string]int{"audit_event": 3})
	assert.Contains(t, buf.String(), "Audit Events")
}

func TestOutputTableHelpers_ExtractFieldName(t *testing.T) {
	t.Parallel()

	require.Equal(t, "title", extractFieldName("Field title does not match pattern"))
	require.Equal(t, "status", extractFieldName("status (Tier 1): invalid enum"))
	require.Equal(t, "", extractFieldName("general non-field error without any field key"))
}
