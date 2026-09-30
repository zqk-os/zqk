package storage

import (
	"testing"
)

// TestEnsureCLIObjectMutationVisible_EmptyFlushKindsDoesNotSkipIndex documents the
// promote/demote CAS miss: nil flushKinds used to skip listing-index FlushKind entirely.
// Empty kinds now FlushAll so index updates survive process exit.
func TestEnsureCLIObjectMutationVisible_EmptyFlushKindsDoesNotSkipIndex(t *testing.T) {
	tmp := t.TempDir()
	f := &FileObjectStorage{projectRoot: tmp}
	ctx, cancel := DurabilityFlushContext()
	defer cancel()
	if err := f.EnsureCLIObjectMutationVisible(ctx, nil); err != nil {
		t.Fatalf("EnsureCLIObjectMutationVisible(nil kinds): %v", err)
	}
	if err := f.EnsureCLIObjectMutationVisible(ctx, []string{}); err != nil {
		t.Fatalf("EnsureCLIObjectMutationVisible(empty kinds): %v", err)
	}
}
