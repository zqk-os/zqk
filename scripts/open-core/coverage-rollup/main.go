// Command coverage-rollup parses or merges Go coverprofiles into a verified package
// coverage telemetry artifact (JSON + sha256 sidecar) for CI upload.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/community"
	"github.com/zqk-os/zqk/pkg/coverage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func main() {
	inPath := flag.String("in", "coverage.out", "path or glob to go coverprofile(s)")
	shards := flag.String("shards", "", "glob pattern or comma-separated list of shard profiles to merge (e.g. 'coverage-*.out')")
	outProfile := flag.String("out", "", "destination path for merged coverprofile (optional)")
	outDir := flag.String("out-dir", ".", "directory for package-coverage-rollup.json (+ .sha256)")
	threshold := flag.Float64("threshold", community.DefaultCoverageThresholdPct, "statement coverage threshold percent")
	failBelow := flag.Bool("fail-below-threshold", false, "exit 1 when coverage is below threshold")
	flag.Parse()

	var report *community.CoverageRollupReport
	var err error

	pattern := strings.TrimSpace(*shards)
	if pattern == "" && (strings.Contains(*inPath, "*") || strings.Contains(*inPath, ",")) {
		pattern = strings.TrimSpace(*inPath)
	}

	if pattern != "" {
		// Multi-shard merging mode
		var files []string
		if strings.Contains(pattern, ",") {
			for _, part := range strings.Split(pattern, ",") {
				p := strings.TrimSpace(part)
				if p != "" {
					files = append(files, p)
				}
			}
		} else {
			matches, globErr := filepath.Glob(pattern)
			if globErr != nil {
				fmt.Fprintf(os.Stderr, "coverage-rollup: invalid glob %q: %v\n", pattern, globErr)
				os.Exit(2)
			}
			files = matches
		}

		if len(files) == 0 {
			fmt.Fprintf(os.Stderr, "coverage-rollup: no shard profiles found matching %q\n", pattern)
			os.Exit(2)
		}
		sort.Strings(files)

		m := coverage.NewMerger()
		merged, mergeErr := m.MergeFiles(files...)
		if mergeErr != nil {
			fmt.Fprintf(os.Stderr, "coverage-rollup: merge: %v\n", mergeErr)
			os.Exit(2)
		}

		if *outProfile != "" {
			if writeErr := fileutil.WriteSecureFile(*outProfile, merged.Format()); writeErr != nil {
				fmt.Fprintf(os.Stderr, "coverage-rollup: write merged profile: %v\n", writeErr)
				os.Exit(2)
			}
		}

		report, err = merged.Rollup(*threshold)
		if err != nil {
			fmt.Fprintf(os.Stderr, "coverage-rollup: rollup: %v\n", err)
			os.Exit(2)
		}
	} else {
		// Single-file mode
		report, err = community.ParseCoverProfile(*inPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "coverage-rollup: parse: %v\n", err)
			os.Exit(2)
		}
		report.ThresholdPct = *threshold
		report.MeetsThreshold = report.CoveragePct >= report.ThresholdPct
	}

	artifactPath, checksumPath, err := community.WriteVerifiedCoverageArtifact(*outDir, report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "coverage-rollup: write: %v\n", err)
		os.Exit(2)
	}
	verified, err := community.VerifyCoverageArtifact(artifactPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "coverage-rollup: verify: %v\n", err)
		os.Exit(2)
	}

	fmt.Printf("schema=%s coverage_pct=%.2f threshold=%.2f meets_threshold=%s packages=%d artifact=%s checksum=%s verified=%s\n",
		verified.Schema,
		verified.CoveragePct,
		verified.ThresholdPct,
		strconv.FormatBool(verified.MeetsThreshold),
		verified.PackageCount,
		filepath.Base(artifactPath),
		filepath.Base(checksumPath),
		strconv.FormatBool(verified.Verified),
	)

	if *failBelow && !verified.MeetsThreshold {
		fmt.Fprintf(os.Stderr, "coverage-rollup: coverage %.2f%% below threshold %.2f%%\n",
			verified.CoveragePct, verified.ThresholdPct)
		os.Exit(1)
	}
}
