package storage

import (
	"path/filepath"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TRACK: REDACTED
func TestSignificantCacheChange_NotePeekConsume(t *testing.T) {
	root := t.TempDir()
	ResetSignificantCacheChangeForTest(root)
	t.Cleanup(func() { ResetSignificantCacheChangeForTest(root) })

	if _, ok := PeekSignificantCacheChange(root); ok {
		t.Fatal("expected no marker initially")
	}
	NoteSignificantCacheChange(root, "unit_test_reason")
	peek, ok := PeekSignificantCacheChange(root)
	if !ok || peek.Reason != "unit_test_reason" {
		t.Fatalf("peek=%v ok=%v", peek, ok)
	}
	if _, err := fileutil.Stat(significantCacheChangePath(root)); err != nil {
		t.Fatalf("marker file missing: %v", err)
	}
	consumed, ok := ConsumeSignificantCacheChange(root)
	if !ok || consumed.Reason != "unit_test_reason" {
		t.Fatalf("consume=%v ok=%v", consumed, ok)
	}
	if _, ok := PeekSignificantCacheChange(root); ok {
		t.Fatal("expected marker consumed")
	}
	if _, err := fileutil.Stat(significantCacheChangePath(root)); !fileutil.IsNotExist(err) {
		t.Fatalf("expected marker file removed, err=%v", err)
	}
}

// TRACK: REDACTED
func TestNoteObjectIDCachePending_BurstNotesSignificantChange(t *testing.T) {
	root := t.TempDir()
	ResetObjectIDCachePendingForTest()
	ResetSignificantCacheChangeForTest(root)
	t.Cleanup(func() {
		ResetObjectIDCachePendingForTest()
		ResetSignificantCacheChangeForTest(root)
	})

	for i := 0; i < SignificantCacheChangePendingThreshold-1; i++ {
		id := "GOAL-" + string(rune('0'+i))
		NoteObjectIDCachePending(root, string(ObjectIDCachePendingOpUpdate), id, "goal",
			filepath.Join(root, "docs/process/goals", id+".yaml"), "test")
	}
	if _, ok := PeekSignificantCacheChange(root); ok {
		t.Fatal("should not note significant change below threshold")
	}
	NoteObjectIDCachePending(root, string(ObjectIDCachePendingOpUpdate), "GOAL-Z", "goal",
		filepath.Join(root, "docs/process/goals/GOAL-Z.yaml"), "test")
	marker, ok := PeekSignificantCacheChange(root)
	if !ok || marker.Reason != SignificantChangeReasonPendingBurst {
		t.Fatalf("expected pending burst marker, got ok=%v marker=%+v", ok, marker)
	}
	if marker.PendingN < SignificantCacheChangePendingThreshold {
		t.Fatalf("pending_count=%d want >= %d", marker.PendingN, SignificantCacheChangePendingThreshold)
	}
}

func TestSignificantChangePreservesValidationCache(t *testing.T) {
	t.Parallel()
	if !SignificantChangePreservesValidationCache(SignificantChangeReasonPendingBurst) {
		t.Fatal("pending burst must preserve validation cache")
	}
	if !SignificantChangePreservesValidationCache(SignificantChangeReasonPendingDrain) {
		t.Fatal("pending drain must preserve validation cache")
	}
	if SignificantChangePreservesValidationCache(SignificantChangeReasonStateRestore) {
		t.Fatal("state restore must still wipe validation cache")
	}
	if SignificantChangePreservesValidationCache(SignificantChangeReasonKindInvalidation) {
		t.Fatal("kind invalidation must still wipe validation cache")
	}
	if SignificantChangePreservesValidationCache("unit_test_reason") {
		t.Fatal("unknown reasons must still wipe validation cache")
	}
}
