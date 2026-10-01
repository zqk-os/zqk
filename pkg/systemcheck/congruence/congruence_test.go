package congruence_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/operational"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/systemcheck/congruence"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestResolveMetricsChunkRetentionDays(t *testing.T) {
	require.Equal(t, 10, congruence.ResolveMetricsChunkRetentionDays(10))
	require.Equal(t, congruence.MaxMetricsChunkRetentionDays, congruence.ResolveMetricsChunkRetentionDays(500))
	require.Equal(t, 30, congruence.ResolveMetricsChunkRetentionDays(0)) // default from config or fallback
}

func TestStreamBackedHelpers(t *testing.T) {
	kinds := congruence.StreamBackedKindsForReport()
	require.NotEmpty(t, kinds)

	dirs := congruence.StreamBackedDirsForReport()
	require.NotEmpty(t, dirs)
}

func TestGatherCacheStatus_EmptyRoot(t *testing.T) {
	tmpDir := t.TempDir()
	status := congruence.GatherCacheStatus(tmpDir)
	require.False(t, status.ObjectIDCache.Loaded)
	require.False(t, status.ReverseReferenceIndex.Loaded)
}

func TestGatherProcessIntegrity_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	integrity := congruence.GatherProcessIntegrity(tmpDir, nil)
	require.Empty(t, integrity.MisplacedByKind)
	require.Empty(t, integrity.UnmanagedFiles)
	require.Empty(t, integrity.UnknownDirs)
	require.Empty(t, integrity.FilesAtRoot)
}

func TestReportWritingAndIndex(t *testing.T) {
	tmpDir := t.TempDir()
	reportDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.LogsDir, paths.LogsReportsSubdir)
	require.NoError(t, fileutil.MkdirAll(reportDir, paths.DirPerm750))

	report := &operational.CongruenceReport{
		GeneratedAt:        time.Now().UTC(),
		ProjectRoot:        tmpDir,
		TotalDisk:          10,
		TotalObject:        10,
		DiskCountByDir:     map[string]int{"tasks": 10},
		ObjectCountByKind:  map[string]int{"task": 10},
		DisparityByDir:     map[string]int{"tasks": 0},
		KindsWithDisparity: nil,
		Alerts:             nil,
	}

	cacheStatus := congruence.CacheStatus{}
	integrity := congruence.ProcessIntegrity{}

	txtPath := filepath.Join(reportDir, "test-report.txt")
	jsonPath := filepath.Join(reportDir, "test-report.json")

	require.NoError(t, congruence.WriteReportFile(txtPath, report, cacheStatus, integrity, nil))
	_, err := os.Stat(txtPath)
	require.NoError(t, err)

	require.NoError(t, congruence.WriteReportJSON(jsonPath, report, cacheStatus, integrity, nil))
	_, err = os.Stat(jsonPath)
	require.NoError(t, err)

	congruence.UpdateReportsIndex(reportDir, jsonPath, report, nil)
	indexPath := filepath.Join(reportDir, paths.ReportsIndexFile)
	_, err = os.Stat(indexPath)
	require.NoError(t, err)

	congruence.WriteDashboardSnapshot(reportDir, report, nil, nil)
	snapshotPath := filepath.Join(reportDir, paths.LatestObjectCountSnapshotFile)
	_, err = os.Stat(snapshotPath)
	require.NoError(t, err)
}

func TestRunCongruence_InvalidRoot(t *testing.T) {
	_, err := congruence.RunCongruence(context.Background(), "", nil, nil, congruence.Options{})
	require.Error(t, err)
}
