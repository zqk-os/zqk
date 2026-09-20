package cas

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

// KindRequiresDescription returns true if the kind mandates a substantive description
// to cross the CAS membrane into storage.
func KindRequiresDescription(kind string) bool {
	return objects.KindRequiresDescription(kind)
}

// isPlaceholderDescription checks if a description string is merely a dummy placeholder.
func isPlaceholderDescription(desc string) bool {
	lower := strings.ToLower(strings.TrimSpace(desc))
	switch lower {
	case "required", "todo", "tbd", "none", "null", "n/a", "placeholder", "title":
		return true
	default:
		return false
	}
}

// ParkObjectWithoutDescription reports whether an object should remain on the draft plane
// because it lacks a valid, substantive description (min 10 characters).
func ParkObjectWithoutDescription(kind string, obj map[string]any) bool {
	if obj == nil {
		return false
	}
	if !KindRequiresDescription(kind) {
		return false
	}
	desc := strings.TrimSpace(objects.GetString(obj, objects.FieldKeyDescription))
	return desc == "" || len(desc) < 10 || isPlaceholderDescription(desc)
}

// RefuseCASWithoutDescription is the CAS membrane gate: hash persist (isDraft=false)
// of an object without a valid description is fail-closed.
func RefuseCASWithoutDescription(kind string, isDraft bool, data []byte) error {
	if isDraft || !KindRequiresDescription(kind) {
		return nil
	}
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return errfmt.Newf("CAS membrane: unmarshal object for description check").Wrap(err)
	}
	objects.CoerceMutationFields(kind, obj)
	desc := strings.TrimSpace(objects.GetString(obj, objects.FieldKeyDescription))
	if desc == "" {
		return errfmt.Errorf("CAS membrane: %s requires a non-empty description to cross into CAS", kind)
	}
	if isPlaceholderDescription(desc) {
		return errfmt.Errorf("CAS membrane: %s description cannot be a placeholder (%q)", kind, desc)
	}
	if len(desc) < 10 {
		return errfmt.Errorf("CAS membrane: %s description is too short (min 10 characters required)", kind)
	}
	return nil
}
