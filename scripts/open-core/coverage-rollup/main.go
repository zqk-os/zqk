// Command coverage-rollup parses a Go coverprofile into a verified package
// coverage telemetry artifact (JSON + sha256 sidecar) for CI upload.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/zqk-os/zqk/pkg/community"
)

func main() {
	inPath := flag.String("in", "coverage.out", "path to go coverprofile")
	outDir := flag.String("out-dir", ".", "directory for package-coverage-rollup.json (+ .sha256)")
	threshold := flag.Float64("threshold", community.DefaultCoverageThresholdPct, "statement coverage threshold percent")
	failBelow := flag.Bool("fail-below-threshold", false, "exit 1 when coverage is below threshold")
	flag.Parse()

	report, err := community.ParseCoverProfile(*inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "coverage-rollup: parse: %v\n", err)
		os.Exit(2)
	}
	report.ThresholdPct = *threshold
	report.MeetsThreshold = report.CoveragePct >= report.ThresholdPct

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
