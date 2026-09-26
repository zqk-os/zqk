package testrunner_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/testrunner"
)

// TestStreamTests_EmptyOrSinglePkg verifies BLI-SCRIPT-PROD-TESTING-002 test progress streaming.
func TestStreamTests_EmptyOrSinglePkg(t *testing.T) {
	outBytes, err := testkit.ManagedCommand(t, t.Context(), "git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	repoRoot := strings.TrimSpace(string(outBytes))

	var buf bytes.Buffer
	opts := testrunner.StreamOptions{
		ProjectRoot: repoRoot,
		Pkg:         "./pkg/systemcheck/policy",
		Parallel:    2,
		Timeout:     30 * time.Second,
	}

	summary, err := testrunner.StreamTests(context.Background(), opts, &buf)
	if err != nil {
		t.Fatalf("stream tests failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "ZQK Test Runner (Progress Streaming)") {
		t.Errorf("expected header in stream output, got: %s", out)
	}
	if !strings.Contains(out, "PASS: pkg/systemcheck/policy") {
		t.Errorf("expected pass notification for pkg/systemcheck/policy, got: %s", out)
	}
	if summary.Passed != 1 {
		t.Errorf("expected 1 passed package, got %d", summary.Passed)
	}
}
