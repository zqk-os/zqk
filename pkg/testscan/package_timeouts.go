package testscan

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

// scanTestsPackageTimeoutsConfig is the on-disk format for scan-tests package minimum timeouts.
type scanTestsPackageTimeoutsConfig struct {
	Packages []struct {
		Pattern    string `yaml:"pattern"`
		MinSeconds int    `yaml:"min_seconds"`
	} `yaml:"packages"`
}

// GetMinTimeoutSecondsForPackage returns the minimum job/test timeout in seconds for the given package path.
// Used by scheduler job generation and SuggestedTimeoutForPackage. Reads from
// .zqk/config/scan_tests_package_timeouts.yaml (override) then config/scan_tests_package_timeouts.yaml (default).
// Longest matching pattern wins. If no config file or no match, returns built-in defaults (600 for cmd/zqk, cmd/zqk/object, pkg/storage; 0 otherwise).
func GetMinTimeoutSecondsForPackage(projectRoot, packagePath string) int {
	normalized := strings.TrimPrefix(packagePath, "./")
	rules := loadScanTestsPackageTimeouts(projectRoot)
	if len(rules) > 0 {
		var best int
		bestLen := -1
		for _, r := range rules {
			if r.Pattern == "" || r.MinSeconds <= 0 {
				continue
			}
			match := normalized == r.Pattern || strings.HasPrefix(normalized, r.Pattern+"/")
			if match && len(r.Pattern) > bestLen {
				best = r.MinSeconds
				bestLen = len(r.Pattern)
			}
		}
		if bestLen >= 0 {
			return best
		}
	}
	return defaultMinTimeoutSecondsForPackage(normalized)
}

func loadScanTestsPackageTimeouts(projectRoot string) []struct {
	Pattern    string
	MinSeconds int
} {
	if projectRoot == "" {
		return nil
	}
	for _, base := range []string{
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.ConfigDir, paths.ScanTestsPackageTimeoutsConfigFile),
		filepath.Join(projectRoot, "config", paths.ScanTestsPackageTimeoutsConfigFile),
	} {
		data, err := os.ReadFile(base)
		if err != nil {
			continue
		}
		var cfg scanTestsPackageTimeoutsConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			continue
		}
		var out []struct {
			Pattern    string
			MinSeconds int
		}
		for _, p := range cfg.Packages {
			out = append(out, struct {
				Pattern    string
				MinSeconds int
			}{Pattern: p.Pattern, MinSeconds: p.MinSeconds})
		}
		return out
	}
	return nil
}

func defaultMinTimeoutSecondsForPackage(packagePath string) int {
	switch {
	case strings.HasPrefix(packagePath, "cmd/zqk/object") || packagePath == "cmd/zqk/object":
		return 600
	case strings.HasPrefix(packagePath, "cmd/zqk") || packagePath == "cmd/zqk":
		return 600
	case strings.HasPrefix(packagePath, "pkg/storage") || packagePath == "pkg/storage":
		return 600
	default:
		return 0
	}
}
