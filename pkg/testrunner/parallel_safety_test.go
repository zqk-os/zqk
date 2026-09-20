package testrunner_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testrunner"
)

func TestScanParallelSafety(t *testing.T) {
	tempDir := t.TempDir()

	testCode := `package sample_test

import (
	"os"
	"testing"
)

func TestParallelAlready(t *testing.T) {
	t.Parallel()
}

func TestUnsafeSetenv(t *testing.T) {
	t.Setenv("FOO", "BAR")
}

func TestSafeTempDir(t *testing.T) {
	tmp := t.TempDir()
	_ = tmp
}

func TestNeedsReview(t *testing.T) {
	// regular logic
}
`
	file := filepath.Join(tempDir, "sample_test.go")
	if err := os.WriteFile(file, []byte(testCode), paths.FilePerm644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	results, err := testrunner.ScanParallelSafety(tempDir, ".")
	if err != nil {
		t.Fatalf("scan parallel safety: %v", err)
	}

	if len(results) != 4 {
		t.Fatalf("expected 4 test results, got %d", len(results))
	}

	levels := make(map[string]testrunner.SafetyLevel)
	for _, r := range results {
		levels[r.FuncName] = r.Level
	}

	if levels["TestParallelAlready"] != testrunner.SafetyLevelParallelAlready {
		t.Errorf("expected TestParallelAlready to be parallel, got %s", levels["TestParallelAlready"])
	}
	if levels["TestUnsafeSetenv"] != testrunner.SafetyLevelUnsafeSetenv {
		t.Errorf("expected TestUnsafeSetenv to be unsafe_setenv, got %s", levels["TestUnsafeSetenv"])
	}
	if levels["TestSafeTempDir"] != testrunner.SafetyLevelSafe {
		t.Errorf("expected TestSafeTempDir to be safe, got %s", levels["TestSafeTempDir"])
	}
	if levels["TestNeedsReview"] != testrunner.SafetyLevelNeedsReview {
		t.Errorf("expected TestNeedsReview to be needs_review, got %s", levels["TestNeedsReview"])
	}
}
