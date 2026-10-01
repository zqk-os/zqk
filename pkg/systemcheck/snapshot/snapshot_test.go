package snapshot

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/systemcheck"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCalculateIssueStatistics(t *testing.T) {
	results := []systemcheck.CheckResult{
		{
			ObjectID:   "BLI-1",
			ObjectKind: "backlog_item",
			Issues: []systemcheck.Issue{
				{Tier: 1, Message: "blocking issue"},
				{Tier: 2, Message: "warning issue"},
			},
		},
		{
			ObjectID:   "BLI-2",
			ObjectKind: "backlog_item",
			Issues: []systemcheck.Issue{
				{Tier: 2, Message: "another warning"},
				{Tier: 3, Message: "informational"},
				{Tier: 4, Message: "recommendation"},
			},
		},
	}

	b, w, i, r := CalculateIssueStatistics(results)
	assert.Equal(t, 1, b)
	assert.Equal(t, 2, w)
	assert.Equal(t, 1, i)
	assert.Equal(t, 1, r)
}

func TestResultsEqual(t *testing.T) {
	r1 := &systemcheck.CheckResult{
		ObjectID:   "OBJ-1",
		ObjectKind: "goal",
		Issues:     []systemcheck.Issue{{Tier: 1}},
	}
	r2 := &systemcheck.CheckResult{
		ObjectID:   "OBJ-1",
		ObjectKind: "goal",
		Issues:     []systemcheck.Issue{{Tier: 1}},
	}
	r3 := &systemcheck.CheckResult{
		ObjectID:   "OBJ-2",
		ObjectKind: "goal",
		Issues:     []systemcheck.Issue{{Tier: 1}},
	}
	r4 := &systemcheck.CheckResult{
		ObjectID:   "OBJ-1",
		ObjectKind: "goal",
		Issues:     []systemcheck.Issue{{Tier: 1}, {Tier: 2}},
	}

	assert.True(t, ResultsEqual(r1, r2))
	assert.False(t, ResultsEqual(r1, r3))
	assert.False(t, ResultsEqual(r1, r4))
	assert.True(t, ResultsEqual(nil, nil))
	assert.False(t, ResultsEqual(r1, nil))
}

func TestCalculateVerifyCount(t *testing.T) {
	assert.Equal(t, 5, CalculateVerifyCount(10, 5))
	assert.Equal(t, 10, CalculateVerifyCount(10, 0))
	assert.Equal(t, 10, CalculateVerifyCount(10, -1))
	assert.Equal(t, 10, CalculateVerifyCount(10, 20))
}

func TestSaveAndLoadCheckSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	results := []systemcheck.CheckResult{
		{
			ObjectID:   "GOAL-1",
			ObjectKind: "goal",
			FilePath:   filepath.Join(tmpDir, "GOAL-1.yaml"),
			Issues: []systemcheck.Issue{
				{Tier: 1, Category: "validation", Message: "title missing", AutoFixable: false},
			},
		},
	}

	err := SaveCheckSnapshotBackground(results, tmpDir, tmpDir, "check", logger)
	require.NoError(t, err)

	latestJSON := filepath.Join(tmpDir, "check-snapshot-latest.json")
	assert.True(t, fileutil.PathExists(latestJSON))

	loaded, err := LoadCheckResults(latestJSON)
	assert.NoError(t, err)
	_ = loaded
}

func TestCompareCheckOutputs(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	baselineFile := filepath.Join(tmpDir, "baseline.jsonl")
	expandedFile := filepath.Join(tmpDir, "expanded.jsonl")
	diffFile := filepath.Join(tmpDir, "diff.json")

	require.NoError(t, fileutil.WriteFile(baselineFile, []byte(
		`{"object_id":"OBJ-1","issues":[{"tier":1}]}`+"\n"+
			`{"object_id":"OBJ-2","issues":[{"tier":2}]}`+"\n",
	), 0644))

	require.NoError(t, fileutil.WriteFile(expandedFile, []byte(
		`{"object_id":"OBJ-2","issues":[]}`+"\n"+
			`{"object_id":"OBJ-3","issues":[{"tier":3}]}`+"\n",
	), 0644))

	err := CompareCheckOutputs(baselineFile, expandedFile, diffFile, logger)
	require.NoError(t, err)

	assert.True(t, fileutil.PathExists(diffFile))
}

func TestConvertResultsToObjects(t *testing.T) {
	results := []systemcheck.CheckResult{
		{
			ObjectID:   "BLI-10",
			ObjectKind: "backlog_item",
			FilePath:   "path/to/BLI-10.yaml",
			AutoFixed:  []string{"rule-1"},
			Issues: []systemcheck.Issue{
				{Tier: 1, Category: "test", Message: "err", AutoFixable: true},
			},
		},
	}

	objs := ConvertResultsToObjects(results)
	require.Len(t, objs, 1)
	assert.Equal(t, "BLI-10", objs[0]["object_id"])
	assert.Equal(t, "backlog_item", objs[0]["kind"])
	assert.Equal(t, []string{"rule-1"}, objs[0]["auto_fixed"])
}
