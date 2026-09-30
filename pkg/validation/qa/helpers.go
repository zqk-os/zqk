package qa

import (
	"github.com/zqk-os/zqk/pkg/objects"
)

// IsObjectComplete reports whether an object map has a complete status.
func IsObjectComplete(obj map[string]any) bool {
	if obj == nil {
		return false
	}
	status, _ := obj[objects.FieldKeyStatus].(string)
	return IsCompleteStatus(status)
}

// hasStringEvidence is a backward compatibility alias for package-internal callers and tests.
func hasStringEvidence(value any) bool {
	return HasStringEvidence(value)
}

// IsTestCaseProven reports whether a test case object map satisfies completion proof.
func IsTestCaseProven(tc map[string]any) bool {
	if tc == nil {
		return false
	}
	tStatus, _ := tc[objects.FieldKeyStatus].(string)
	remOpen, _ := tc["remaining_open_count"].(int)
	return IsCompleteStatus(tStatus) && remOpen == 0
}

