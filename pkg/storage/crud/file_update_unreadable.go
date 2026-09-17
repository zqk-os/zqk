package crud

import (
	"strings"
)

const ConstStreamFailedToUnmarshalObjectAfterCasDiscovery = "failed to unmarshal object after CAS discovery"

func LiveCASBlobUnreadable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "STREAM_FAILED_TO_UNMARSHAL_OBJECT_AFTER_CAS_DISCOVERY") ||
		strings.Contains(msg, "STREAM_FAILED_TO_UNMARSHAL_OBJECT") ||
		strings.Contains(msg, ConstStreamFailedToUnmarshalObjectAfterCasDiscovery) ||
		strings.Contains(msg, "content hash mismatch:") ||
		strings.Contains(msg, "failed to parse YAML")
}
