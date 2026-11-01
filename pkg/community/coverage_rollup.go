package community

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Schema and artifact identifiers for package coverage rollups.
const (
	CoverageRollupSchema        = "zqk_package_coverage_rollup_v1"
	CoverageArtifactFilename    = "package-coverage-rollup.json"
	CoverageRollupStepName      = "package_coverage_rollup"
	DefaultCoverageThresholdPct = 85.0
	CoverageModulePrefix        = "github.com/zqk-os/zqk/"
	RepoResolverPersonaID       = "PER-1793541871378023000-273387ff"
)

// Coverage band thresholds mirror scripts/open-core/generate-coverage-summary.py.
const (
	CoverageBandHigh   = "high"
	CoverageBandMedium = "medium"
	CoverageBandLow    = "low"
	coverageHighFloor  = 80.0
	coverageMedFloor   = 50.0
)

// Domain errors for coverage rollup parsing and artifact verification.
var (
	ErrCoverageProfileMissing   = errors.New("coverage rollup: cover profile missing")
	ErrCoverageProfileEmpty     = errors.New("coverage rollup: cover profile produced no statement records")
	ErrCoverageArtifactTampered = errors.New("coverage rollup: artifact checksum mismatch")
	ErrNilCoverageReport        = errors.New("coverage rollup: nil report")
)

// PackageCoverageEntry is the per-package statement coverage rollup.
type PackageCoverageEntry struct {
	Package     string  `json:"package"`
	Statements  int     `json:"statements"`
	Covered     int     `json:"covered"`
	CoveragePct float64 `json:"coverage_pct"`
	Band        string  `json:"band"`
}

// CoverageRollupReport is the verified telemetry artifact for statement coverage.
type CoverageRollupReport struct {
	Schema              string                 `json:"schema"`
	GeneratedAt         time.Time              `json:"generated_at"`
	SourceProfile       string                 `json:"source_profile,omitempty"`
	SourceProfileSHA256 string                 `json:"source_profile_sha256"`
	TotalStatements     int                    `json:"total_statements"`
	CoveredStatements   int                    `json:"covered_statements"`
	CoveragePct         float64                `json:"coverage_pct"`
	ThresholdPct        float64                `json:"threshold_pct"`
	MeetsThreshold      bool                   `json:"meets_threshold"`
	PackageCount        int                    `json:"package_count"`
	HighCount           int                    `json:"high_count"`
	MediumCount         int                    `json:"medium_count"`
	LowCount            int                    `json:"low_count"`
	Packages            []PackageCoverageEntry `json:"packages"`
	ArtifactSHA256      string                 `json:"artifact_sha256,omitempty"`
	Verified            bool                   `json:"verified,omitempty"`
}

// CoverageBandForPct maps a statement coverage percentage onto high/medium/low.
func CoverageBandForPct(pct float64) string {
	switch {
	case pct >= coverageHighFloor:
		return CoverageBandHigh
	case pct >= coverageMedFloor:
		return CoverageBandMedium
	default:
		return CoverageBandLow
	}
}

// ParseCoverProfile parses a Go coverprofile path into a package rollup report.
func ParseCoverProfile(profilePath string) (*CoverageRollupReport, error) {
	if !fileutil.Exists(profilePath) {
		return nil, fmt.Errorf("%w: %s", ErrCoverageProfileMissing, profilePath)
	}
	data, err := fileutil.ReadFile(profilePath)
	if err != nil {
		return nil, fmt.Errorf("coverage rollup: read profile: %w", err)
	}
	report, err := ParseCoverProfileBytes(data)
	if err != nil {
		return nil, err
	}
	report.SourceProfile = filepath.Base(profilePath)
	return report, nil
}

// ParseCoverProfileBytes parses coverprofile bytes into a package rollup report.
func ParseCoverProfileBytes(data []byte) (*CoverageRollupReport, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, ErrCoverageProfileEmpty
	}

	type counters struct {
		total   int
		covered int
	}
	packages := map[string]*counters{}
	totalStmts := 0
	coveredStmts := 0

	scanner := bufio.NewScanner(bytes.NewReader(data))
	// Cover profiles can contain long paths; raise the token limit.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		stmts, err := strconv.Atoi(parts[len(parts)-2])
		if err != nil || stmts <= 0 {
			continue
		}
		count, err := strconv.Atoi(parts[len(parts)-1])
		if err != nil {
			continue
		}
		loc := strings.Join(parts[:len(parts)-2], " ")
		filePath := strings.SplitN(loc, ":", 2)[0]
		cleanPath := strings.TrimPrefix(filePath, CoverageModulePrefix)
		pkgName := filepath.ToSlash(filepath.Dir(cleanPath))
		if pkgName == "." || pkgName == "" {
			pkgName = filepath.ToSlash(cleanPath)
		}

		c := packages[pkgName]
		if c == nil {
			c = &counters{}
			packages[pkgName] = c
		}
		c.total += stmts
		totalStmts += stmts
		if count > 0 {
			c.covered += stmts
			coveredStmts += stmts
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("coverage rollup: scan profile: %w", err)
	}
	if totalStmts == 0 || len(packages) == 0 {
		return nil, ErrCoverageProfileEmpty
	}

	entries := make([]PackageCoverageEntry, 0, len(packages))
	high, med, low := 0, 0, 0
	for name, c := range packages {
		pct := 0.0
		if c.total > 0 {
			pct = (float64(c.covered) / float64(c.total)) * 100.0
		}
		band := CoverageBandForPct(pct)
		switch band {
		case CoverageBandHigh:
			high++
		case CoverageBandMedium:
			med++
		default:
			low++
		}
		entries = append(entries, PackageCoverageEntry{
			Package:     name,
			Statements:  c.total,
			Covered:     c.covered,
			CoveragePct: pct,
			Band:        band,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Package < entries[j].Package
	})

	totalPct := (float64(coveredStmts) / float64(totalStmts)) * 100.0
	sum := sha256.Sum256(data)

	return &CoverageRollupReport{
		Schema:              CoverageRollupSchema,
		GeneratedAt:         time.Now().UTC(),
		SourceProfileSHA256: hex.EncodeToString(sum[:]),
		TotalStatements:     totalStmts,
		CoveredStatements:   coveredStmts,
		CoveragePct:         totalPct,
		ThresholdPct:        DefaultCoverageThresholdPct,
		MeetsThreshold:      totalPct >= DefaultCoverageThresholdPct,
		PackageCount:        len(entries),
		HighCount:           high,
		MediumCount:         med,
		LowCount:            low,
		Packages:            entries,
	}, nil
}

// WriteVerifiedCoverageArtifact writes the rollup JSON and a sidecar sha256 checksum.
// Returns the artifact path and checksum path.
func WriteVerifiedCoverageArtifact(dir string, report *CoverageRollupReport) (string, string, error) {
	if report == nil {
		return "", "", ErrNilCoverageReport
	}
	if strings.TrimSpace(dir) == "" {
		return "", "", fmt.Errorf("coverage rollup: output directory is required")
	}
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return "", "", fmt.Errorf("coverage rollup: mkdir: %w", err)
	}

	// Persist without artifact hash/verified flags; then stamp hash of canonical bytes.
	toWrite := *report
	toWrite.ArtifactSHA256 = ""
	toWrite.Verified = false
	if toWrite.Schema == "" {
		toWrite.Schema = CoverageRollupSchema
	}
	if toWrite.GeneratedAt.IsZero() {
		toWrite.GeneratedAt = time.Now().UTC()
	}
	if toWrite.ThresholdPct <= 0 {
		toWrite.ThresholdPct = DefaultCoverageThresholdPct
	}
	toWrite.MeetsThreshold = toWrite.CoveragePct >= toWrite.ThresholdPct

	payload, err := json.MarshalIndent(&toWrite, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("coverage rollup: marshal: %w", err)
	}
	payload = append(payload, '\n')
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	toWrite.ArtifactSHA256 = digest
	toWrite.Verified = true

	finalPayload, err := json.MarshalIndent(&toWrite, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("coverage rollup: marshal stamped: %w", err)
	}
	finalPayload = append(finalPayload, '\n')

	artifactPath := filepath.Join(dir, CoverageArtifactFilename)
	if err := fileutil.WriteFile(artifactPath, finalPayload, paths.FilePerm644); err != nil {
		return "", "", fmt.Errorf("coverage rollup: write artifact: %w", err)
	}

	// Sidecar checksum covers the on-disk bytes (including stamped fields).
	onDiskSum := sha256.Sum256(finalPayload)
	onDiskDigest := hex.EncodeToString(onDiskSum[:])
	checksumPath := artifactPath + ".sha256"
	checksumLine := fmt.Sprintf("%s  %s\n", onDiskDigest, CoverageArtifactFilename)
	if err := fileutil.WriteFile(checksumPath, []byte(checksumLine), paths.FilePerm644); err != nil {
		return "", "", fmt.Errorf("coverage rollup: write checksum: %w", err)
	}

	report.ArtifactSHA256 = onDiskDigest
	report.Verified = true
	report.MeetsThreshold = toWrite.MeetsThreshold
	report.ThresholdPct = toWrite.ThresholdPct
	return artifactPath, checksumPath, nil
}

// VerifyCoverageArtifact validates the JSON artifact against its sidecar sha256 file.
func VerifyCoverageArtifact(artifactPath string) (*CoverageRollupReport, error) {
	data, err := fileutil.ReadFile(artifactPath)
	if err != nil {
		return nil, fmt.Errorf("coverage rollup: read artifact: %w", err)
	}
	checksumPath := artifactPath + ".sha256"
	checksumBytes, err := fileutil.ReadFile(checksumPath)
	if err != nil {
		return nil, fmt.Errorf("coverage rollup: read checksum: %w", err)
	}
	fields := strings.Fields(string(checksumBytes))
	if len(fields) < 1 {
		return nil, fmt.Errorf("%w: empty checksum file", ErrCoverageArtifactTampered)
	}
	expected := strings.ToLower(fields[0])
	actualSum := sha256.Sum256(data)
	actual := hex.EncodeToString(actualSum[:])
	if !strings.EqualFold(expected, actual) {
		return nil, fmt.Errorf("%w: expected %s got %s", ErrCoverageArtifactTampered, expected, actual)
	}

	var report CoverageRollupReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("coverage rollup: decode artifact: %w", err)
	}
	if report.Schema != CoverageRollupSchema {
		return nil, fmt.Errorf("coverage rollup: unexpected schema %q", report.Schema)
	}
	report.Verified = true
	report.ArtifactSHA256 = actual
	return &report, nil
}

// CoverageRollupToStageMetric converts a rollup report into a pipeline telemetry stage metric.
func CoverageRollupToStageMetric(report *CoverageRollupReport, jobID, pipelineID, artifactPath string) (PipelineStageMetric, error) {
	if report == nil {
		return PipelineStageMetric{}, ErrNilCoverageReport
	}
	if strings.TrimSpace(jobID) == "" {
		return PipelineStageMetric{}, fmt.Errorf("%w: job_id is required", ErrMissingRequiredField)
	}

	outcome := OutcomeFailed
	if report.MeetsThreshold {
		outcome = OutcomeSuccess
	}

	meta := map[string]string{
		"schema":                CoverageRollupSchema,
		"coverage_pct":          fmt.Sprintf("%.4f", report.CoveragePct),
		"threshold_pct":         fmt.Sprintf("%.4f", report.ThresholdPct),
		"meets_threshold":       strconv.FormatBool(report.MeetsThreshold),
		"package_count":         strconv.Itoa(report.PackageCount),
		"total_statements":      strconv.Itoa(report.TotalStatements),
		"covered_statements":    strconv.Itoa(report.CoveredStatements),
		"source_profile_sha256": report.SourceProfileSHA256,
		"high_count":            strconv.Itoa(report.HighCount),
		"medium_count":          strconv.Itoa(report.MediumCount),
		"low_count":             strconv.Itoa(report.LowCount),
	}
	if artifactPath != "" {
		meta["artifact_path"] = artifactPath
	}
	if report.ArtifactSHA256 != "" {
		meta["artifact_sha256"] = report.ArtifactSHA256
	}

	now := time.Now().UTC()
	return PipelineStageMetric{
		PipelineID:      pipelineID,
		JobID:           jobID,
		StepName:        CoverageRollupStepName,
		Outcome:         outcome,
		StartedAt:       now,
		CompletedAt:     now,
		ThroughputItems: report.PackageCount,
		Metadata:        meta,
	}, nil
}

// IngestCoverageRollup records a coverage rollup as a pipeline stage on the convergence session.
func (s *RepoResolverConvergenceSession) IngestCoverageRollup(report *CoverageRollupReport, jobID, pipelineID, artifactPath string) error {
	metric, err := CoverageRollupToStageMetric(report, jobID, pipelineID, artifactPath)
	if err != nil {
		return err
	}
	return s.IngestMetric(metric)
}
