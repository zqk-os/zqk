package scheduler

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// StreamSummaryTestBundleHealthAlias is the stable path alias for test-bundle health.jsonl (REQ-DATASTREAM-001 pilot).
const StreamSummaryTestBundleHealthAlias = "scheduler_test_bundles_health_jsonl"

// TestBundleHealthStreamSummary is a bounded, queryable summary row for the logical health stream.
type TestBundleHealthStreamSummary struct {
	PathAlias        string `json:"path_alias"`
	Kind             string `json:"kind"`
	LogicalPath      string `json:"logical_path"`
	ByteSize         int64  `json:"byte_size"`
	LineCount        int    `json:"line_count"`
	UpdatedAtRFC3339 string `json:"updated_at_rfc3339"`
}

// WriteTestBundleHealthStreamSummary writes a JSON summary for test-bundles/health.jsonl under
// <projectRoot>/.zqk/stream_summary/ (single file; bucketing applied if more stream kinds are added).
func WriteTestBundleHealthStreamSummary(projectRoot string) (TestBundleHealthStreamSummary, error) {
	var out TestBundleHealthStreamSummary
	if projectRoot == emptyValue {
		return out, errfmt.Errorf("project root required")
	}
	logical := TestBundlesHealthFilePath(projectRoot)
	st, err := os.Stat(logical)
	if err != nil {
		return out, errfmt.Newf("stat health file").Wrap(err)
	}
	nLines := 0
	f, err := os.Open(logical)
	if err != nil {
		return out, errfmt.Newf("open health file").Wrap(err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	// Bound work for very large files (summary is approximate line count for observability).
	const maxScanLines = 500000
	for sc.Scan() && nLines < maxScanLines {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		nLines++
	}
	if err := sc.Err(); err != nil {
		return out, errfmt.Newf("scan health file").Wrap(err)
	}

	out = TestBundleHealthStreamSummary{
		PathAlias:        StreamSummaryTestBundleHealthAlias,
		Kind:             "test_bundle_health",
		LogicalPath:      logical,
		ByteSize:         st.Size(),
		LineCount:        nLines,
		UpdatedAtRFC3339: st.ModTime().UTC().Format(time.RFC3339),
	}

	destDir := filepath.Join(projectRoot, paths.ProjectDataDir, "stream_summary")
	if err := fileutil.EnsureDir(destDir); err != nil {
		return out, errfmt.Newf("mkdir stream_summary").Wrap(err)
	}
	dest := filepath.Join(destDir, "test_bundle_health.json")
	payload, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return out, errfmt.Newf("marshal summary").Wrap(err)
	}
	if err := fileutil.WriteSecureFile(dest, payload); err != nil {
		return out, errfmt.Newf("write summary").Wrap(err)
	}
	return out, nil
}
