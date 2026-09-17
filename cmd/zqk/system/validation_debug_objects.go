package system

import (
	"strings"
	"sync"

	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/when"
)

var (
	debugObjectIDs     map[string]bool
	debugObjectIDsOnce sync.Once
)

// isDebugValidationObject returns true if the given object ID is in the set of IDs
// that receive extra debug logging during validation. The set is read once from
// the environment variable ZQK_DEBUG_VALIDATION_OBJECT_IDS (comma-separated).
// If unset or empty, no IDs receive special logging.
func isDebugValidationObject(id string) bool {
	if when.IsEmpty(id) {
		return false
	}
	debugObjectIDsOnce.Do(func() {
		raw := config.ValidationDebugValidationObjectIDs().OrDefault("")
		if when.IsEmpty(raw) {
			debugObjectIDs = nil
			return
		}
		debugObjectIDs = make(map[string]bool)
		for s := range strings.SplitSeq(raw, ",") {
			s = strings.TrimSpace(s)
			if !when.IsEmpty(s) {
				debugObjectIDs[s] = true
			}
		}
	})
	if debugObjectIDs == nil {
		return false
	}
	return debugObjectIDs[id]
}
