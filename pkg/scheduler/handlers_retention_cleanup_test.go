package scheduler

import (
	"testing"
)

func TestRetentionCleanup_Smoke(t *testing.T) {
	t.Parallel()
	// Constructor returns RetentionToleranceHandlerInterface; a typed
	// &RetentionToleranceHandler{} is never nil, so that check was impossible.
	handler := NewRetentionToleranceHandler(nil, "")
	if handler == nil {
		t.Fatal("expected non-nil RetentionToleranceHandler")
	}
}
