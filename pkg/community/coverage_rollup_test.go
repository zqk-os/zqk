package community_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/community"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestCoverageRollup_ParseAndVerifyArtifact satisfies CRIT-1793575220034202000-1b185e5b:
// automated package coverage rollups emit a verified statement-coverage telemetry artifact.
func TestCoverageRollup_ParseAndVerifyArtifact(t *testing.T) {
	t.Parallel()

	profile := strings.Join([]string{
		"mode: atomic",
		"github.com/zqk-os/zqk/pkg/community/coverage_rollup.go:10.2,20.3 5 1",
		"github.com/zqk-os/zqk/pkg/community/coverage_rollup.go:21.2,30.3 5 0",
		"github.com/zqk-os/zqk/pkg/paths/constants.go:1.1,10.2 10 10",
		"github.com/zqk-os/zqk/cmd/zqk/main.go:1.1,5.2 2 0",
		"",
	}, "\n")

	dir := t.TempDir()
	profilePath := filepath.Join(dir, "coverage.out")
	require.NoError(t, fileutil.WriteFile(profilePath, []byte(profile), paths.FilePerm644))

	report, err := community.ParseCoverProfile(profilePath)
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Equal(t, community.CoverageRollupSchema, report.Schema)
	assert.Equal(t, 22, report.TotalStatements)
	assert.Equal(t, 15, report.CoveredStatements)
	assert.InDelta(t, 68.1818, report.CoveragePct, 0.01)
	assert.Equal(t, 3, report.PackageCount)
	assert.Equal(t, 1, report.HighCount)   // pkg/paths @ 100%
	assert.Equal(t, 1, report.MediumCount) // pkg/community @ 50%
	assert.Equal(t, 1, report.LowCount)    // cmd/zqk @ 0%
	assert.False(t, report.MeetsThreshold)
	assert.Equal(t, community.DefaultCoverageThresholdPct, report.ThresholdPct)
	require.NotEmpty(t, report.SourceProfileSHA256)
	require.Len(t, report.Packages, 3)

	byPkg := map[string]community.PackageCoverageEntry{}
	for _, entry := range report.Packages {
		byPkg[entry.Package] = entry
	}
	assert.Equal(t, community.CoverageBandMedium, byPkg["pkg/community"].Band)
	assert.Equal(t, community.CoverageBandHigh, byPkg["pkg/paths"].Band)
	assert.Equal(t, community.CoverageBandLow, byPkg["cmd/zqk"].Band)

	artifactPath, checksumPath, err := community.WriteVerifiedCoverageArtifact(dir, report)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, community.CoverageArtifactFilename), artifactPath)
	assert.Equal(t, artifactPath+".sha256", checksumPath)
	require.FileExists(t, artifactPath)
	require.FileExists(t, checksumPath)

	verified, err := community.VerifyCoverageArtifact(artifactPath)
	require.NoError(t, err)
	require.NotNil(t, verified)
	assert.True(t, verified.Verified)
	assert.Equal(t, report.SourceProfileSHA256, verified.SourceProfileSHA256)
	assert.Equal(t, report.CoveragePct, verified.CoveragePct)

	raw, err := fileutil.ReadFile(artifactPath)
	require.NoError(t, err)
	var decoded community.CoverageRollupReport
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, community.CoverageRollupSchema, decoded.Schema)
}

func TestCoverageRollup_FailClosedOnMissingOrEmpty(t *testing.T) {
	t.Parallel()

	_, err := community.ParseCoverProfile(filepath.Join(t.TempDir(), "missing.out"))
	require.Error(t, err)
	assert.ErrorIs(t, err, community.ErrCoverageProfileMissing)

	emptyPath := filepath.Join(t.TempDir(), "empty.out")
	require.NoError(t, fileutil.WriteFile(emptyPath, []byte("mode: atomic\n"), paths.FilePerm644))
	_, err = community.ParseCoverProfile(emptyPath)
	require.Error(t, err)
	assert.ErrorIs(t, err, community.ErrCoverageProfileEmpty)

	_, err = community.ParseCoverProfileBytes(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, community.ErrCoverageProfileEmpty)
}

func TestCoverageRollup_VerifyDetectsTamper(t *testing.T) {
	t.Parallel()

	profile := "mode: set\ngithub.com/zqk-os/zqk/pkg/community/a.go:1.1,2.2 4 4\n"
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "coverage.out")
	require.NoError(t, fileutil.WriteFile(profilePath, []byte(profile), paths.FilePerm644))

	report, err := community.ParseCoverProfile(profilePath)
	require.NoError(t, err)
	report.ThresholdPct = 50
	report.MeetsThreshold = report.CoveragePct >= report.ThresholdPct

	artifactPath, _, err := community.WriteVerifiedCoverageArtifact(dir, report)
	require.NoError(t, err)

	tampered := []byte(`{"schema":"zqk_package_coverage_rollup_v1","total_statements":1}`)
	require.NoError(t, fileutil.WriteFile(artifactPath, tampered, paths.FilePerm644))

	_, err = community.VerifyCoverageArtifact(artifactPath)
	require.Error(t, err)
	assert.ErrorIs(t, err, community.ErrCoverageArtifactTampered)
}

func TestCoverageRollup_IngestIntoPipelineTelemetry(t *testing.T) {
	t.Parallel()

	profile := "mode: atomic\ngithub.com/zqk-os/zqk/pkg/community/a.go:1.1,2.2 8 8\n"
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "coverage.out")
	require.NoError(t, fileutil.WriteFile(profilePath, []byte(profile), paths.FilePerm644))

	report, err := community.ParseCoverProfile(profilePath)
	require.NoError(t, err)
	report.ThresholdPct = 85
	report.MeetsThreshold = report.CoveragePct >= report.ThresholdPct
	assert.True(t, report.MeetsThreshold)

	artifactPath, _, err := community.WriteVerifiedCoverageArtifact(dir, report)
	require.NoError(t, err)

	session, err := community.NewRepoResolverConvergenceSession(
		"cov-sess-1",
		community.RepoResolverPersonaID,
		nil,
	)
	require.NoError(t, err)

	metric, err := community.CoverageRollupToStageMetric(report, "job-cov-1", "pipe-cov", artifactPath)
	require.NoError(t, err)
	assert.Equal(t, community.CoverageRollupStepName, metric.StepName)
	assert.Equal(t, community.OutcomeSuccess, metric.Outcome)
	assert.Equal(t, artifactPath, metric.Metadata["artifact_path"])
	assert.Equal(t, "true", metric.Metadata["meets_threshold"])
	assert.Equal(t, report.ArtifactSHA256, metric.Metadata["artifact_sha256"])

	metric.CompletedAt = time.Now().UTC()
	metric.StartedAt = metric.CompletedAt.Add(-time.Millisecond)
	require.NoError(t, session.IngestMetric(metric))

	matrix := session.TelemetryEngine.GetConvergenceMatrix()
	require.NotNil(t, matrix)
	assert.Equal(t, 1, matrix.TotalSteps)
	assert.Equal(t, 1, matrix.SuccessCount)
	assert.Contains(t, matrix.StepBreakdown, community.CoverageRollupStepName)
}

func TestCoverageRollup_BelowThresholdMarksFailedOutcome(t *testing.T) {
	t.Parallel()

	report := &community.CoverageRollupReport{
		Schema:              community.CoverageRollupSchema,
		TotalStatements:     100,
		CoveredStatements:   50,
		CoveragePct:         50,
		ThresholdPct:        85,
		MeetsThreshold:      false,
		PackageCount:        1,
		SourceProfileSHA256: "abc",
		Packages: []community.PackageCoverageEntry{{
			Package: "pkg/x", Statements: 100, Covered: 50, CoveragePct: 50, Band: community.CoverageBandMedium,
		}},
	}
	metric, err := community.CoverageRollupToStageMetric(report, "job-2", "pipe-2", "")
	require.NoError(t, err)
	assert.Equal(t, community.OutcomeFailed, metric.Outcome)
}
