package objectget

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/validation"
)

// InferKindFromObjectID resolves the object kind from an ID prefix (via id_prefixes_config and specs),
// matching zqk CLI behavior for object routing.
//
// If projectRoot is non-empty, ID patterns are loaded for that repo layout
// (.zqk/specs/objects relative to projectRoot).
// If projectRoot is empty, uses validation.GetIDValidator() with environment discovery (same as many in-process callers).
//
// Trimmed empty id returns "". Unknown prefixes return "".
func InferKindFromObjectID(projectRoot, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}

	var v *validation.IDValidator
	if projectRoot != "" {
		specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
		v = validation.NewIDValidator(specsDir)
	} else {
		v = validation.GetIDValidator()
	}

	_ = v.LoadPatterns()
	return v.InferKindFromID(id)
}
