package scheduler

import (
	"testing"
)

func TestRetentionMaxCount_IsHighVolumeKind(t *testing.T) {
	// Smoke test for isHighVolumeKind
	res := isHighVolumeKind("audit_event")
	_ = res
}
