package testscan

import (
	"strings"

	"github.com/zqk-os/zqk/internal/testpackageconcurrency"
)

// PackageWantsGoTestParallelOne is true when bundled `go test` should pass -parallel 1
// (subprocess / flake sensitivity). Used by scan-tests when building go test args and when
// computing how many concurrent bundle jobs may target the same package.
func PackageWantsGoTestParallelOne(packagePath string) bool {
	switch strings.TrimPrefix(strings.TrimSpace(packagePath), "./") {
	case "cmd/zqk/scheduler":
		return true
	default:
		return false
	}
}

// ComputePackageConcurrencyLimitsFromBundles returns, for each package path appearing in bundles,
// the maximum number of concurrent run_wrapper jobs (separate go test processes) that may run
// for that package. Derived only from scan context: sequential tests and package policy force 1;
// otherwise uses maxParallel (clamped).
func ComputePackageConcurrencyLimitsFromBundles(bundles []*TestBundle, maxParallel int) map[string]int {
	if maxParallel < 1 {
		maxParallel = 4
	}
	if maxParallel > 32 {
		maxParallel = 32
	}

	byPkgTests := make(map[string][]*TestFunction)
	for _, b := range bundles {
		pp := testpackageconcurrency.NormalizePackagePath(b.PackagePath)
		if pp == "" {
			continue
		}
		byPkgTests[pp] = append(byPkgTests[pp], b.Tests...)
	}

	out := make(map[string]int, len(byPkgTests))
	for pkg, tests := range byPkgTests {
		if PackageWantsGoTestParallelOne(pkg) {
			out[pkg] = 1
			continue
		}
		serial := false
		for _, tf := range tests {
			if tf != nil && !tf.IsParallel {
				serial = true
				break
			}
		}
		if serial {
			out[pkg] = 1
			continue
		}
		out[pkg] = maxParallel
	}
	return out
}
