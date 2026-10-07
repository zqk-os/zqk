package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
)

func newTestCommandWithBuffer() (*cobra.Command, *bytes.Buffer) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	return cmd, &buf
}

func TestOutputBulkResult_NilResult(t *testing.T) {
	cmd := &cobra.Command{}
	// Should not panic or error
	OutputBulkResult(cmd, nil, "table", "update")
}

func TestOutputBulkResult_SuccessAndErrors(t *testing.T) {
	cmd, buf := newTestCommandWithBuffer()

	res := &storage.BulkResult{
		TotalCount:   2,
		SuccessCount: 1,
		FailureCount: 1,
		Results:      []map[string]any{{"id": "BLI-1"}},
		Errors: []storage.BulkOperationError{
			{ID: "BLI-2", Index: 1, Message: "failed validation", Error: errors.New("failed validation")},
		},
	}

	OutputBulkResult(cmd, res, "yaml", "update")
	out := buf.String()
	if !strings.Contains(out, "BLI-1") || !strings.Contains(out, "BLI-2") {
		t.Fatalf("expected output to contain IDs, got: %s", out)
	}
}

func TestOutputBulkResult_FallbackFormat(t *testing.T) {
	cmd, buf := newTestCommandWithBuffer()
	cmd.Flags().String("format", "unknown_format_xyz", "")

	res := &storage.BulkResult{
		TotalCount:   1,
		SuccessCount: 1,
		FailureCount: 0,
		Results:      []map[string]any{{"id": "BLI-9"}},
	}

	OutputBulkResult(cmd, res, "table", "create")
	out := buf.String()
	if !strings.Contains(out, "BLI-9") {
		t.Fatalf("expected output to contain BLI-9 from fallback, got: %s", out)
	}
}

func TestOutputBulkResult_LastResortFallback(t *testing.T) {
	cmd, buf := newTestCommandWithBuffer()
	cmd.Flags().String("format", "unknown_format_xyz", "")

	res := &storage.BulkResult{
		TotalCount: 1,
	}

	OutputBulkResult(cmd, res, "unsupported_legacy_format", "create")
	out := buf.String()
	if !strings.Contains(out, "operation: create") {
		t.Fatalf("expected legacy fallback output, got: %s", out)
	}
}
