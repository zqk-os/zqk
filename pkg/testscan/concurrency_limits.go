package testscan

import (
	"encoding/json"
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/internal/testpackageconcurrency"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// PackageConcurrencyLimitsFileName is stored under .zqk/test-bundles/; scan-tests updates it so the
// scheduler can enforce the same cross-process limits for any package without hardcoded paths.
const PackageConcurrencyLimitsFileName = testpackageconcurrency.LimitsFileName

const packageConcurrencySchemaVersion = 1

type packageConcurrencyFile struct {
	SchemaVersion              int            `json:"schema_version"`
	UpdatedAt                  string         `json:"updated_at"`
	MaxConcurrentJobsByPackage map[string]int `json:"max_concurrent_jobs_by_package"`
}

// ReadPackageConcurrencyLimitsMap loads max concurrent run_wrapper jobs per package from disk.
// Missing file returns (nil, nil). Invalid JSON returns an error.
func ReadPackageConcurrencyLimitsMap(projectRoot string) (map[string]int, error) {
	return testpackageconcurrency.ReadLimitsMap(projectRoot)
}

// WritePackageConcurrencyLimitsPatch merges limits for packages touched by this scan into the
// on-disk map (other package keys are preserved). Call after bundles are known and before jobs run.
func WritePackageConcurrencyLimitsPatch(projectRoot string, bundles []*TestBundle, maxParallel int) error {
	if projectRoot == "" {
		return nil
	}
	patch := ComputePackageConcurrencyLimitsFromBundles(bundles, maxParallel)
	if len(patch) == 0 {
		return nil
	}
	dir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.TestBundlesDir)
	if err := fileutil.EnsureDir(dir); err != nil {
		return errfmt.Newf("mkdir test-bundles").Wrap(err)
	}
	path := filepath.Join(dir, PackageConcurrencyLimitsFileName)
	existing, _ := testpackageconcurrency.ReadLimitsMap(projectRoot)
	if existing == nil {
		existing = make(map[string]int)
	}
	for k, v := range patch {
		existing[testpackageconcurrency.NormalizePackagePath(k)] = v
	}
	payload := packageConcurrencyFile{
		SchemaVersion:              packageConcurrencySchemaVersion,
		UpdatedAt:                  zqktime.NowRFC3339UTC(),
		MaxConcurrentJobsByPackage: existing,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	// Unique temp name: concurrent scan-tests invocations share this path; a fixed ".tmp"
	// sibling races (ENOENT on rename when another process already renamed the same tmp).
	tmpFile, err := fileutil.CreateTemp(dir, "pclimits-*.tmp")
	if err != nil {
		return errfmt.Newf("create temp for package concurrency limits").Wrap(err)
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = fileutil.Remove(tmpPath)
		return errfmt.Newf("write temp package concurrency limits").Wrap(err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = fileutil.Remove(tmpPath)
		return errfmt.Newf("close temp package concurrency limits").Wrap(err)
	}
	if err := fileutil.Chmod(tmpPath, 0o600); err != nil {
		_ = fileutil.Remove(tmpPath)
		return errfmt.Newf("chmod temp package concurrency limits").Wrap(err)
	}
	if err := fileutil.Rename(tmpPath, path); err != nil {
		_ = fileutil.Remove(tmpPath)
		return err
	}
	return nil
}

// MergePackageConcurrencyLimitMaps returns a new map with per-key minimum of a and b (when both set).
// Keys present in only one map are included. Used to combine on-disk policy with job metadata.
func MergePackageConcurrencyLimitMaps(a, b map[string]int) map[string]int {
	return testpackageconcurrency.MergeLimitMaps(a, b)
}
