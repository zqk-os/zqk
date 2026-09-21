package testscan

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// scanTestsPackageTimeoutsConfig is the on-disk format for scan-tests package minimum timeouts.
type scanTestsPackageTimeoutsConfig struct {
	Packages []struct {
		Pattern    string `yaml:"pattern"`
		MinSeconds int    `yaml:"min_seconds"`
	} `yaml:"packages"`
}

type packageTimeoutRule struct {
	Pattern    string
	MinSeconds int
}

var scanTimeouts stampmemo.Table[[]packageTimeoutRule] // keyed by projectRoot; stamp is override+default YAML

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

func scanTestsTimeoutPaths(projectRoot string) (override, fallback string) {
	override = filepath.Join(projectRoot, paths.ProjectDataDir, paths.ConfigDir, paths.ScanTestsPackageTimeoutsConfigFile)
	fallback = filepath.Join(projectRoot, "config", paths.ScanTestsPackageTimeoutsConfigFile)
	return override, fallback
}

func loadScanTestsPackageTimeouts(projectRoot string) []packageTimeoutRule {
	if projectRoot == "" {
		return nil
	}
	override, fallback := scanTestsTimeoutPaths(projectRoot)
	rules, _ := scanTimeouts.Load(projectRoot, stampmemo.OfAll(override, fallback), func() ([]packageTimeoutRule, error) {
		return readScanTestsPackageTimeouts(override, fallback), nil
	})
	return rules
}

func readScanTestsPackageTimeouts(paths ...string) []packageTimeoutRule {
	for _, base := range paths {
		data, err := fileutil.ReadFile(base)
		if err != nil {
			continue
		}
		var cfg scanTestsPackageTimeoutsConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			continue
		}
		out := make([]packageTimeoutRule, 0, len(cfg.Packages))
		for _, p := range cfg.Packages {
			out = append(out, packageTimeoutRule{Pattern: p.Pattern, MinSeconds: p.MinSeconds})
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
