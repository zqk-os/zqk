package filecas

import (
	"errors"
	"testing"
)

func TestErrObjectNotFound(t *testing.T) {
	t.Parallel()
	if ErrObjectNotFound == nil {
		t.Fatal("ErrObjectNotFound is nil")
	}
	if ErrObjectNotFound.Error() != "object not found" {
		t.Fatalf("ErrObjectNotFound=%q", ErrObjectNotFound.Error())
	}
	if !errors.Is(ErrObjectNotFound, ErrObjectNotFound) {
		t.Fatal("errors.Is identity failed")
	}
}

func TestIndexWriteQueueVarIsOptional(t *testing.T) {
	t.Parallel()
	// Extracted wiring leaves these nil until pkg/storage assigns them.
	if GlobalIndexWriteQueue != nil && GetMetrics == nil {
		t.Fatal("unexpected half-wired metrics vs queue")
	}
}
