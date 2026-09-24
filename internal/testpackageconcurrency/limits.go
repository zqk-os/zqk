// Package testpackageconcurrency holds on-disk package concurrency policy for test bundles
// (legacy SCH-run-* run_wrapper jobs) without importing pkg/testing.
package testpackageconcurrency

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// LimitsFileName is stored under .zqk/test-bundles/; leftover from retired bundle scans so the
// scheduler can enforce the same cross-process limits for any package without hardcoded paths.
const LimitsFileName = "package_concurrency_limits.json"

type limitsFile struct {
	SchemaVersion              int            `json:"schema_version"`
	UpdatedAt                  string         `json:"updated_at"`
	MaxConcurrentJobsByPackage map[string]int `json:"max_concurrent_jobs_by_package"`
}

// NormalizePackagePath trims "./" and surrounding space for map keys and lookups.
func NormalizePackagePath(packagePath string) string {
	return strings.TrimPrefix(strings.TrimSpace(packagePath), "./")
}

// ReadLimitsMap loads max concurrent run_wrapper jobs per package from disk.
// Missing file returns (nil, nil). Invalid JSON returns an error.
func ReadLimitsMap(projectRoot string) (map[string]int, error) {
	if projectRoot == "" {
		return nil, nil
	}
	p := filepath.Join(projectRoot, paths.ProjectDataDir, paths.TestBundlesDir, LimitsFileName)
	data, err := fileutil.ReadFile(p)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var f limitsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, errfmt.Errorf("parse %s: %w", p, err)
	}
	if f.MaxConcurrentJobsByPackage == nil {
		return map[string]int{}, nil
	}
	out := make(map[string]int, len(f.MaxConcurrentJobsByPackage))
	for k, v := range f.MaxConcurrentJobsByPackage {
		kk := NormalizePackagePath(k)
		if kk == "" || v <= 0 {
			continue
		}
		if v > 32 {
			v = 32
		}
		out[kk] = v
	}
	return out, nil
}

// MergeLimitMaps returns a new map with per-key minimum of a and b (when both set).
// Keys present in only one map are included. Used to combine on-disk policy with job metadata.
func MergeLimitMaps(a, b map[string]int) map[string]int {
	out := make(map[string]int)
	for k, v := range a {
		if v > 0 {
			out[k] = v
		}
	}
	for k, v := range b {
		if v <= 0 {
			continue
		}
		if cur, ok := out[k]; !ok || v < cur {
			out[k] = v
		}
	}
	return out
}
