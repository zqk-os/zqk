package storage

import (
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// isIdentityRewriteField reports whether a field stores an account/object identity
// that must be rewritten on rename in addition to *_ref / *_refs.
// TRACK: BLI-1785905134201010000-07393484 — ACC-* full cutover; keep in sync with rename.
func isIdentityRewriteField(fieldName string) bool {
	switch fieldName {
	case objects.FieldKeyCreatedBy, objects.FieldKeyUpdatedBy, objects.FieldKeyAccountID:
		return true
	default:
		return strings.HasSuffix(fieldName, "_ref") || strings.HasSuffix(fieldName, "_refs")
	}
}

// collectRewritableIdentityFields returns field names on obj that are identity-rewrite targets.
func collectRewritableIdentityFields(obj map[string]any) []string {
	if obj == nil {
		return nil
	}
	out := make([]string, 0, 8)
	for fieldName := range obj {
		if isIdentityRewriteField(fieldName) {
			out = append(out, fieldName)
		}
	}
	return out
}
