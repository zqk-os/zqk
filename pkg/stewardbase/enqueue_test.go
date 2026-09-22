package stewardbase

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
)

func TestEnqueueStewardMaintenance(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	logger := logging.NewLogger(&buf, logging.InfoLevel, logging.NewJSONFormatter(ctx))

	// Error case: unknown storage profile
	err := EnqueueStewardMaintenance(ctx, "/tmp", datacell.StorageProfile("unknown_profile"), datacell.MaintenanceOp{}, logger)
	if err == nil {
		t.Fatalf("expected error for unknown storage profile")
	}

	// Error case: empty projectRoot
	err = EnqueueStewardMaintenance(ctx, "", datacell.ProfileCASEntity, datacell.MaintenanceOp{}, logger)
	if err == nil {
		t.Fatalf("expected error for empty project root")
	}

	// Success case with truncation of long detail
	tmpDir := t.TempDir()
	longDetail := strings.Repeat("a", StewardEnqueueDetailMaxBytes+100)
	op := datacell.MaintenanceOp{
		Name:   "vacuum",
		Detail: longDetail,
	}

	err = EnqueueStewardMaintenance(ctx, tmpDir, datacell.ProfileCASEntity, op, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Success case with logger = nil
	err = EnqueueStewardMaintenance(ctx, tmpDir, datacell.ProfileCASEntity, datacell.MaintenanceOp{Name: "compact"}, nil)
	if err != nil {
		t.Fatalf("unexpected error with nil logger: %v", err)
	}
}

func TestAppendStewardEnqueueRecord_Errors(t *testing.T) {
	// Empty project root
	err := AppendStewardEnqueueRecord("", map[string]any{"key": "val"})
	if err == nil {
		t.Fatalf("expected error for empty project root")
	}

	err = AppendStewardEnqueueRecord(".", map[string]any{"key": "val"})
	if err == nil {
		t.Fatalf("expected error for '.' project root")
	}

	// Unserializable record
	err = AppendStewardEnqueueRecord(t.TempDir(), map[string]any{
		"invalid": make(chan int),
	})
	if err == nil {
		t.Fatalf("expected marshal error")
	}
}
