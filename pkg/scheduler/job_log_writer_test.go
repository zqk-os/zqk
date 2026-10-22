package scheduler

import (
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestTrackAndAppendTestBundleProgress(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// TrackAndAppendTestBundleProgress is a deprecated safe no-op.
	// It should never panic, and should not create or scan progress.jsonl files.
	TrackAndAppendTestBundleProgress(tmpDir, "SCH-run-bundle-test", "started")
	TrackAndAppendTestBundleProgress(tmpDir, "SCH-run-bundle-test", "passed")
	TrackAndAppendTestBundleProgress("", "", "")

	progressPath := TestBundlesProgressFilePath(tmpDir)
	if fileutil.Exists(progressPath) {
		t.Fatalf("expected progress file not to be created by no-op: %s", progressPath)
	}
}
