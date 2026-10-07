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

func TestOutputBulkResult_NilResult(t *testing.T) {
	cmd := &cobra.Command{}
	// Should not panic or error
	OutputBulkResult(cmd, nil, "table", "update")
}

func TestOutputBulkResult_SuccessAndErrors(t *testing.T) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)

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
