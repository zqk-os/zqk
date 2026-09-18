package precommit

import (
	"bytes"
	"context"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestStatus_MissingResultsFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	cmd := NewStatusCmd()
	cmd.SetArgs([]string{"--project-root", dir})
	out := bytes.NewBuffer(nil)
	// WriteOutput uses logging.GetCommandOutputWriter(ctx), not SetOut alone.
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), out))
	cmd.SetOut(out)
	cmd.SetErr(out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("status with missing results file should not error: %v", err)
	}
	got := out.String()
	if got == emptyValue {
		t.Error("expected status output when results file is missing")
	}
	if !strings.Contains(got, "not found") && !strings.Contains(got, "Required action") {
		t.Errorf("expected output to mention not found or required action, got: %s", got)
	}
}
