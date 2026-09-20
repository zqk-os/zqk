package filecas

import "testing"

func TestCASIndexWireConstants(t *testing.T) {
	t.Parallel()
	if emptyValue != "" {
		t.Fatalf("emptyValue=%q want empty string", emptyValue)
	}
	if storageCASIndexWirePrefix != "storage_cas_index" {
		t.Fatalf("storageCASIndexWirePrefix=%q", storageCASIndexWirePrefix)
	}
	if CASIndexFormatVersion != "1" {
		t.Fatalf("CASIndexFormatVersion=%q want 1", CASIndexFormatVersion)
	}
	if ErrMsgHashMismatchVerify != "hash mismatch: expected %s, got %s" {
		t.Fatalf("ErrMsgHashMismatchVerify=%q", ErrMsgHashMismatchVerify)
	}
	if ObjectIDCachePendingOpInvalidate != "invalidate" || ObjectIDCachePendingOpUpdate != "update" {
		t.Fatalf("pending op constants drifted: invalidate=%q update=%q", ObjectIDCachePendingOpInvalidate, ObjectIDCachePendingOpUpdate)
	}
}
