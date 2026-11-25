package interceptor

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestDetectZgrepCompliant(t *testing.T) {
	t.Parallel()

	assert.True(t, DetectZgrepCompliant(`zgrep "HandleRequest" pkg/`))
	assert.True(t, DetectZgrepCompliant(`./bin/zqk grep --ast --kind struct pkg/`))
	assert.True(t, DetectZgrepCompliant(`zqk grep "foo" --max-tokens 2000 -f json`))
	assert.False(t, DetectZgrepCompliant(`grep -rn "HandleRequest" pkg/`))
	assert.False(t, DetectZgrepCompliant(`find . -name "*.go"`))
	assert.False(t, DetectZgrepCompliant(``))
}

func TestDetectRawFindNonCompliance(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// Repo-scoped find triggers steering nudge
	hit, nudge := DetectRawFindNonCompliance(`find pkg/ -name "*.go"`, root)
	assert.True(t, hit)
	assert.Contains(t, nudge, "GH-1220")
	assert.Contains(t, nudge, "--max-tokens 2000")

	hitDot, nudgeDot := DetectRawFindNonCompliance(`find . -type f`, root)
	assert.True(t, hitDot)
	assert.NotEmpty(t, nudgeDot)

	// External /tmp find bypasses
	hitTmp, nudgeTmp := DetectRawFindNonCompliance(`find /tmp/scratch -name "*.log"`, root)
	assert.False(t, hitTmp)
	assert.Empty(t, nudgeTmp)

	// Non-find command bypasses
	hitOther, _ := DetectRawFindNonCompliance(`git status -sb`, root)
	assert.False(t, hitOther)
}

func TestRecordAndSummarizeCompliance_Concurrent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		idx := i
		goroutinelabels.NewGoroutine("compliance_test_writer", "concurrent compliance telemetry write").StartSimple(func() {
			defer wg.Done()
			ev := ComplianceEvent{
				Kind:                 ComplianceKindZgrep,
				Tool:                 "zgrep",
				Compliant:            idx%2 == 0,
				CommandLine:          `zgrep "Foo" pkg/`,
				EstimatedTokensSaved: 6000,
			}
			if !ev.Compliant {
				ev.Kind = ComplianceKindRawSearchIntercepted
				ev.Tool = "grep"
			}
			require.NoError(t, RecordComplianceEvent(root, ev))
		})
	}
	wg.Wait()

	summary, err := SummarizeCompliance(root)
	require.NoError(t, err)
	assert.Equal(t, 6, summary.TotalSearchInvocations)
	assert.Equal(t, 3, summary.ZgrepCompliantCount)
	assert.Equal(t, 3, summary.RawSearchNonCompliantCount)
	assert.InDelta(t, 50.0, summary.ComplianceRatioPct, 0.01)
	assert.Equal(t, 36000, summary.EstimatedTokensSaved)
}
