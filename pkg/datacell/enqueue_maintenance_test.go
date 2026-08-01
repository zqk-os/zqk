package datacell

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestEnqueueStewardMaintenance_appendsJSONL(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ctx := context.Background()
	if err := EnqueueStewardMaintenance(ctx, root, ProfileStream, MaintenanceOp{Name: MaintenanceOpRefreshSummary, Detail: "unit"}, nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(StewardEnqueueJSONLPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), MaintenanceOpRefreshSummary) {
		t.Fatalf("jsonl %q", string(b))
	}
}

func TestEnqueueStewardMaintenance_streamStewardKind_appendsJSONL(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ctx := context.Background()
	detail := "c=cycle-x|k=audit_event|p=segments"
	if err := EnqueueStewardMaintenance(ctx, root, ProfileStream,
		MaintenanceOp{Name: MaintenanceOpStreamStewardKind, Detail: detail}, nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(StewardEnqueueJSONLPath(root))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, MaintenanceOpStreamStewardKind) || !strings.Contains(s, detail) {
		t.Fatalf("jsonl want op+detail; got %q", s)
	}
}

func TestEnqueueStewardMaintenance_unknownProfile(t *testing.T) {
	t.Parallel()
	err := EnqueueStewardMaintenance(context.Background(), t.TempDir(), StorageProfile("nope"),
		MaintenanceOp{Name: MaintenanceOpRefreshSummary}, nil)
	if err == nil {
		t.Fatal("want error")
	}
}

func TestNewStewardMaintenanceCoordinator_unknownProfile(t *testing.T) {
	t.Parallel()
	if _, err := NewStewardMaintenanceCoordinator(t.TempDir(), StorageProfile("x"), nil); err == nil {
		t.Fatal("want error")
	}
}

func TestStewardMaintenanceCoordinator_Enqueue(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	c, err := NewStewardMaintenanceCoordinator(root, ProfileCASEntity, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Enqueue(context.Background(), MaintenanceOp{Name: MaintenanceOpRetentionSweep, Detail: "x"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(StewardEnqueueJSONLPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), MaintenanceOpRetentionSweep) {
		t.Fatalf("jsonl %q", string(b))
	}
}

func TestStewardMaintenanceCoordinator_nilSafe(t *testing.T) {
	t.Parallel()
	var c *StewardMaintenanceCoordinator
	if err := c.Enqueue(context.Background(), MaintenanceOp{Name: "x"}); err != nil {
		t.Fatal(err)
	}
}

func TestTruncateStewardEnqueueDetail(t *testing.T) {
	t.Parallel()
	s := strings.Repeat("a", StewardEnqueueDetailMaxBytes+50)
	got := truncateStewardEnqueueDetail(s)
	if len(got) != StewardEnqueueDetailMaxBytes {
		t.Fatalf("len %d", len(got))
	}
}
