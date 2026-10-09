package snapshot

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/systemcheck"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExpandCheckSnapshot_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// 1. Success case with >= 1000 items to exercise Sync
	inputPath := filepath.Join(tmpDir, "input.jsonl")
	outputPath := filepath.Join(tmpDir, "output.jsonl")

	var sb strings.Builder
	// Write 1005 valid lines
	for i := 0; i < 1005; i++ {
		res := systemcheck.CheckResult{
			ObjectID:   fmt.Sprintf("OBJ-%d", i),
			ObjectKind: "goal",
			Issues: []systemcheck.Issue{
				{Tier: 1, Message: fmt.Sprintf("issue %d", i)},
			},
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		sb.Write(data)
		sb.WriteByte('\n')
	}

	require.NoError(t, fileutil.WriteFile(inputPath, []byte(sb.String()), 0644))

	// Run background execution synchronously
	err := ExpandCheckSnapshotBackground(inputPath, outputPath, logger)
	require.NoError(t, err)

	// Verify expanded file content
	results, err := LoadCheckResults(outputPath)
	require.NoError(t, err)
	assert.Len(t, results, 1005)

	// Test ExpandCheckSnapshot (goroutine launcher)
	outBg := filepath.Join(tmpDir, "output_bg.jsonl")
	err = ExpandCheckSnapshot(inputPath, outBg, logger)
	require.NoError(t, err)
	// Give background routine a brief moment to run
	time.Sleep(50 * time.Millisecond)

	// Error cases
	// Nonexistent input file
	err = ExpandCheckSnapshotBackground(filepath.Join(tmpDir, "nonexistent.jsonl"), outputPath, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open snapshot file")

	// Invalid output file (directory path)
	err = ExpandCheckSnapshotBackground(inputPath, tmpDir, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create output file")

	// EnrichCheckResult
	singleRes := &systemcheck.CheckResult{ObjectID: "TEST-1"}
	enriched := EnrichCheckResult(singleRes)
	assert.Equal(t, singleRes, enriched)
}

func TestSaveCheckSnapshot_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	results := []systemcheck.CheckResult{
		{
			ObjectID:   "GOAL-1",
			ObjectKind: "goal",
			FilePath:   "path/to/GOAL-1.yaml",
			AutoFixed:  []string{"autofix-1"},
			Issues: []systemcheck.Issue{
				{Tier: 1, Category: "cat1", Message: "blocking", AutoFixable: true},
				{Tier: 2, Category: "cat2", Message: "warning", AutoFixable: false},
				{Tier: 3, Category: "cat3", Message: "info", AutoFixable: false},
				{Tier: 4, Category: "cat4", Message: "recommendation", AutoFixable: false},
			},
		},
	}

	// 1. Background launcher
	snapDir := filepath.Join(tmpDir, "snapshots")
	err := SaveCheckSnapshot(results, snapDir, tmpDir, "check", logger)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// 2. Synchronous save
	err = SaveCheckSnapshotBackground(results, snapDir, tmpDir, "check", logger)
	require.NoError(t, err)

	latestJSON := filepath.Join(snapDir, "check-snapshot-latest.json")
	assert.True(t, fileutil.Exists(latestJSON))
	latestCsnap := filepath.Join(snapDir, "check-snapshot-latest.csnap")
	assert.True(t, fileutil.Exists(latestCsnap))

	// 3. Error case: snapshotPath is an existing file, so MkdirAll fails
	conflictFile := filepath.Join(tmpDir, "conflict_file")
	require.NoError(t, fileutil.WriteFile(conflictFile, []byte("file"), 0644))
	err = SaveCheckSnapshotBackground(results, conflictFile, tmpDir, "check", logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create snapshot directory")
}

func TestLoadCheckSnapshot_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Nonexistent file
	_, err := LoadCheckSnapshot(filepath.Join(tmpDir, "missing.json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read snapshot file")

	// 2. Corrupt JSON file
	corruptFile := filepath.Join(tmpDir, "corrupt.json")
	require.NoError(t, fileutil.WriteFile(corruptFile, []byte("{not json"), 0644))
	_, err = LoadCheckSnapshot(corruptFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal snapshot")

	// 3. Valid JSON snapshot
	validFile := filepath.Join(tmpDir, "valid.json")
	snap := systemcheck.CheckSnapshot{
		Metadata: systemcheck.CheckSnapshotMetadata{
			TotalObjects: 1,
			TotalIssues:  2,
		},
		Results: []systemcheck.CheckResult{
			{ObjectID: "BLI-1"},
		},
	}
	data, err := json.Marshal(snap)
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(validFile, data, 0644))

	loaded, err := LoadCheckSnapshot(validFile)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, 1, loaded.Metadata.TotalObjects)
	assert.Equal(t, 2, loaded.Metadata.TotalIssues)
	assert.Len(t, loaded.Results, 1)
	assert.Equal(t, "BLI-1", loaded.Results[0].ObjectID)
}

func TestReadAndExpandSnapshot_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// 1. Nonexistent file
	_, err := ReadAndExpandSnapshot(filepath.Join(tmpDir, "missing.csnap"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read compressed snapshot")

	_, _, err = ReadAndExpandSnapshotWithHeader(filepath.Join(tmpDir, "missing.csnap"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read compressed snapshot")

	// 2. Corrupt file
	corruptFile := filepath.Join(tmpDir, "corrupt.csnap")
	require.NoError(t, fileutil.WriteFile(corruptFile, []byte("bad compressed snapshot header"), 0644))
	_, err = ReadAndExpandSnapshot(corruptFile)
	require.Error(t, err)

	// 3. Valid compressed snapshot
	objs := []map[string]any{
		{
			"object_id":   "GOAL-100",
			"object_kind": "goal",
			"issues":      []map[string]any{},
		},
	}
	now := time.Now().UTC()
	cs, err := storage.CreateCompressedSnapshot(objs, now, logger)
	require.NoError(t, err)

	csnapPath := filepath.Join(tmpDir, "valid.csnap")
	err = storage.WriteCompressedSnapshot(cs, csnapPath)
	require.NoError(t, err)

	// Read with header
	headerCS, expanded, err := ReadAndExpandSnapshotWithHeader(csnapPath)
	require.NoError(t, err)
	require.NotNil(t, headerCS)
	require.Len(t, expanded, 1)
	assert.Equal(t, "GOAL-100", expanded[0]["object_id"])

	// Read direct
	expandedDirect, err := ReadAndExpandSnapshot(csnapPath)
	require.NoError(t, err)
	require.Len(t, expandedDirect, 1)
	assert.Equal(t, "GOAL-100", expandedDirect[0]["object_id"])
}

func TestCompareCheckOutputs_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	baselinePath := filepath.Join(tmpDir, "baseline.jsonl")
	expandedPath := filepath.Join(tmpDir, "expanded.jsonl")
	diffPath := filepath.Join(tmpDir, "diff.json")

	// Missing baseline
	err := CompareCheckOutputs(filepath.Join(tmpDir, "missing_baseline.jsonl"), expandedPath, diffPath, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load baseline")

	// Write baseline
	baseRes := []byte(
		`{"object_id":"OBJ-1","issues":[{"tier":1}]}` + "\n" +
			`{"object_id":"OBJ-2","issues":[{"tier":2}]}` + "\n" +
			`{"object_id":"OBJ-3","issues":[{"tier":1}]}` + "\n",
	)
	require.NoError(t, fileutil.WriteFile(baselinePath, baseRes, 0644))

	// Missing expanded
	err = CompareCheckOutputs(baselinePath, filepath.Join(tmpDir, "missing_expanded.jsonl"), diffPath, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load expanded")

	// Write expanded: OBJ-1 modified, OBJ-2 unchanged, OBJ-3 removed, OBJ-4 added
	expRes := []byte(
		`{"object_id":"OBJ-1","issues":[{"tier":1},{"tier":2}]}` + "\n" +
			`{"object_id":"OBJ-2","issues":[{"tier":2}]}` + "\n" +
			`{"object_id":"OBJ-4","issues":[{"tier":4}]}` + "\n",
	)
	require.NoError(t, fileutil.WriteFile(expandedPath, expRes, 0644))

	// Write to invalid diff directory
	err = CompareCheckOutputs(baselinePath, expandedPath, filepath.Join(tmpDir, "no_dir", "sub", "diff.json"), logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write diff file")

	// Successful comparison
	err = CompareCheckOutputs(baselinePath, expandedPath, diffPath, logger)
	require.NoError(t, err)

	data, err := fileutil.ReadFile(diffPath)
	require.NoError(t, err)

	var diff CheckDiff
	require.NoError(t, json.Unmarshal(data, &diff))
	assert.Equal(t, 3, diff.BaselineCount)
	assert.Equal(t, 3, diff.ExpandedCount)
	assert.Len(t, diff.Added, 1)
	assert.Equal(t, "OBJ-4", diff.Added[0].ObjectID)
	assert.Len(t, diff.Removed, 1)
	assert.Equal(t, "OBJ-3", diff.Removed[0].ObjectID)
	assert.Len(t, diff.Modified, 1)
	assert.Equal(t, "OBJ-1", diff.Modified[0].ObjectID)
}

func TestLoadCheckResults_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()

	// Missing file
	_, err := LoadCheckResults(filepath.Join(tmpDir, "missing.jsonl"))
	require.Error(t, err)

	// Valid lines with empty lines
	linesFile := filepath.Join(tmpDir, "lines.jsonl")
	content := "{\"object_id\":\"VALID-1\"}\n{\"object_id\":\"VALID-2\"}\n"
	require.NoError(t, fileutil.WriteFile(linesFile, []byte(content), 0644))

	results, err := LoadCheckResults(linesFile)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "VALID-1", results[0].ObjectID)
	assert.Equal(t, "VALID-2", results[1].ObjectID)
}
