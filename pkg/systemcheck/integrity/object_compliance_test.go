package integrity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestClassifyObjectComplianceTrend(t *testing.T) {
	tests := []struct {
		name     string
		delta    *ObjectComplianceDelta
		expected string
	}{
		{"nil delta", nil, "unknown"},
		{"blocking decreased", &ObjectComplianceDelta{BlockingIssues: -1, TotalIssues: 5}, "improving"},
		{"blocking same, total decreased", &ObjectComplianceDelta{BlockingIssues: 0, TotalIssues: -2}, "improving"},
		{"blocking increased", &ObjectComplianceDelta{BlockingIssues: 1, TotalIssues: -10}, "worsening"},
		{"blocking same, total increased", &ObjectComplianceDelta{BlockingIssues: 0, TotalIssues: 3}, "worsening"},
		{"no change", &ObjectComplianceDelta{BlockingIssues: 0, TotalIssues: 0}, "flat"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := ClassifyObjectComplianceTrend(tc.delta)
			assert.Equal(t, tc.expected, res)
		})
	}
}

func TestAppendAndReadObjectComplianceHistory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zqk-compliance-hist-*")
	require.NoError(t, err)
	defer fileutil.RemoveAll(tmpDir)

	histPath := KernelObjectComplianceHistoryPath(tmpDir)

	e1 := ObjectComplianceHistoryEntry{
		MeasuredAt:     "2026-10-01T01:00:00Z",
		SourcePath:     "path/1",
		TotalIssues:    10,
		BlockingIssues: 2,
	}
	e2 := ObjectComplianceHistoryEntry{
		MeasuredAt:     "2026-10-01T02:00:00Z",
		SourcePath:     "path/2",
		TotalIssues:    8,
		BlockingIssues: 1,
	}

	require.NoError(t, AppendObjectComplianceHistory(tmpDir, e1))
	require.NoError(t, AppendObjectComplianceHistory(tmpDir, e2))

	// Duplicate append should be skipped
	require.NoError(t, AppendObjectComplianceHistory(tmpDir, e2))

	tail := ReadObjectComplianceHistoryTail(histPath, 5)
	require.Len(t, tail, 2)
	assert.Equal(t, "path/1", tail[0].SourcePath)
	assert.Equal(t, "path/2", tail[1].SourcePath)

	last := ReadLastObjectComplianceHistory(histPath)
	require.NotNil(t, last)
	assert.Equal(t, "path/2", last.SourcePath)

	penultimate := ReadPenultimateObjectComplianceHistory(histPath)
	require.NotNil(t, penultimate)
	assert.Equal(t, "path/1", penultimate.SourcePath)
}

func TestLoadObjectComplianceSnapshot(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zqk-compliance-load-*")
	require.NoError(t, err)
	defer fileutil.RemoveAll(tmpDir)

	// No summary exists
	snap := LoadObjectComplianceSnapshot(tmpDir)
	assert.False(t, snap.Available)
	assert.Equal(t, "unknown", snap.Trend)

	// Write compact check JSON
	precommitDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.PreCommitDir)
	require.NoError(t, fileutil.MkdirAll(precommitDir, paths.DirPerm755))

	checkFile := filepath.Join(precommitDir, "system-check.json")
	summaryData := CheckSummaryFile{}
	summaryData.Summary.TotalObjects = 50
	summaryData.Summary.TotalIssues = 3
	summaryData.Summary.BlockingIssues = 0
	summaryData.Summary.PendingAutofixBatches = 0
	b, err := json.Marshal(summaryData)
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(checkFile, b, paths.FilePerm644))

	past := time.Now().Add(-2 * time.Hour)
	_ = fileutil.Chtimes(checkFile, past, past)

	snap2 := LoadObjectComplianceSnapshot(tmpDir)
	assert.True(t, snap2.Available)
	assert.True(t, snap2.ObjectComplianceOK)
	assert.Equal(t, 50, snap2.TotalObjects)
	assert.Equal(t, 3, snap2.TotalIssues)
	assert.Equal(t, 0, snap2.BlockingIssues)

	// Update with new sample to verify trend
	summaryData.Summary.TotalIssues = 1
	b2, err := json.Marshal(summaryData)
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(checkFile, b2, paths.FilePerm644))
	now := time.Now()
	_ = fileutil.Chtimes(checkFile, now, now)

	snap3 := LoadObjectComplianceSnapshot(tmpDir)
	assert.True(t, snap3.Available)
	assert.Equal(t, 1, snap3.TotalIssues)
	assert.Equal(t, "improving", snap3.Trend)
	require.NotNil(t, snap3.Delta)
	assert.Equal(t, -2, snap3.Delta.TotalIssues)
}
