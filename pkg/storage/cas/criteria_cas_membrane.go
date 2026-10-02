package cas

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

// ValidateCriteriaBaseInstance checks that a criteria object satisfies base
// instance validation required to cross the CAS membrane:
// 1. Valid category from the criteria spec enum.
// 2. Non-empty title (min 5 chars, single-line, non-placeholder).
// 3. Substantive description (min 10 chars, non-placeholder).
// 4. Valid criteria lifecycle status.
func ValidateCriteriaBaseInstance(kind string, obj map[string]any) error {
	if kind != objects.KindCriteria {
		return nil
	}
	if obj == nil {
		return errfmt.Errorf("criteria object cannot be nil")
	}
	if err := objects.RequireCriteriaCategory(kind, obj); err != nil {
		return err
	}
	title := strings.TrimSpace(objects.GetString(obj, objects.FieldKeyTitle))
	if title == "" {
		return errfmt.Errorf("criteria requires a non-empty title")
	}
	if len(title) < 5 {
		return errfmt.Errorf("criteria title %q is too short (min 5 characters)", title)
	}
	if strings.Contains(title, "\n") {
		return errfmt.Errorf("criteria title must be single line")
	}
	lowerTitle := strings.ToLower(title)
	if lowerTitle == "required" || lowerTitle == "todo" || lowerTitle == "title" || lowerTitle == "placeholder" {
		return errfmt.Errorf("criteria title cannot be placeholder %q", title)
	}

	desc := strings.TrimSpace(objects.GetString(obj, objects.FieldKeyDescription))
	if desc == "" {
		return errfmt.Errorf("criteria requires a non-empty description (min 10 characters)")
	}
	if len(desc) < 10 {
		return errfmt.Errorf("criteria description is too short (min 10 characters)")
	}
	if isPlaceholderDescription(desc) {
		return errfmt.Errorf("criteria description cannot be placeholder %q", desc)
	}

	status := strings.TrimSpace(objects.GetString(obj, objects.FieldKeyStatus))
	if status != "" {
		switch status {
		case "conceptual", "originated", "awaiting_verification", "in_progress", "validated", "complete", "blocked", "rejected", "archived":
			// valid lifecycle status
		default:
			return errfmt.Errorf("criteria status %q is not a valid criteria lifecycle status", status)
		}
	}
	return nil
}

// ParkCriteriaWithoutBaseInstance is true when kind is criteria and it fails
// base instance validation. Those objects stay on the draft plane; hash CAS
// must not receive them.
func ParkCriteriaWithoutBaseInstance(kind string, obj map[string]any) bool {
	return ValidateCriteriaBaseInstance(kind, obj) != nil
}

// ParkCriteriaWithoutCategory is true when kind is criteria and category is
// empty after alias remap or fails base instance validation.
func ParkCriteriaWithoutCategory(kind string, obj map[string]any) bool {
	return ParkCriteriaWithoutBaseInstance(kind, obj)
}

// UseObjectDraftPlane decides draft vs hash CAS for a live object map.
// Incomplete criteria park off CAS unless the caller asked for promote
// (CLI --promote / WithPromoteOnCreate), in which case the CAS membrane
// refuses the write instead of failing create-validate.
func UseObjectDraftPlane(kind string, obj map[string]any, promoteOnCreate bool) bool {
	if obj == nil {
		return false
	}
	if objects.IsBypassKind(kind) {
		return false
	}
	status := objects.GetString(obj, objects.FieldKeyStatus)
	should := shouldUseObjectDraftPlane(kind, status)

	if should {
		return true
	}
	if promoteOnCreate {
		return false
	}
	if ParkCriteriaWithoutBaseInstance(kind, obj) {
		return true
	}
	return ParkObjectWithoutDescription(kind, obj)
}

// RefuseCriteriaCASWithoutBaseInstance is the CAS membrane: hash persist
// (isDraft=false) of criteria failing base instance validation is fail-closed.
func RefuseCriteriaCASWithoutBaseInstance(kind string, isDraft bool, data []byte) error {
	if isDraft || kind != objects.KindCriteria {
		return nil
	}
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return errfmt.Newf("CAS membrane: unmarshal criteria").Wrap(err)
	}
	objects.CoerceMutationFields(kind, obj)
	return ValidateCriteriaBaseInstance(kind, obj)
}

// RefuseCriteriaCASWithoutCategory is the CAS membrane: hash persist
// (isDraft=false) of criteria without category or failing base instance validation is fail-closed.
func RefuseCriteriaCASWithoutCategory(kind string, isDraft bool, data []byte) error {
	return RefuseCriteriaCASWithoutBaseInstance(kind, isDraft, data)
}

// ShouldUseObjectDraftPlane is true for CAS kinds in a preliminary lifecycle status.
func ShouldUseObjectDraftPlane(kind, status string) bool {
	// Must match pkg/storage.shouldUseObjectDraftPlane for non-stream kinds:
	// lifecycle preliminary (exploring/identified/draft/…) parks on the draft plane.
	// The previous draft|draft_pending literal left backlog_item exploring on CAS.
	// TRACK: draft-plane create / promote membrane.
	if kind == "" || status == "" {
		return false
	}
	if objects.IsBypassKind(kind) {
		return false
	}
	checker := objects.GetGlobalStatusChecker()
	if checker.IsPreliminary(kind, status) {
		return true
	}
	loader := objects.GetGlobalLifecycleLoader()
	if loader == nil {
		return false
	}
	origin, err := loader.GetOriginStatus(kind)
	if err != nil || origin == "" {
		return false
	}
	return status == origin && checker.IsPreliminary(kind, origin)
}

func shouldUseObjectDraftPlane(kind, status string) bool {
	return ShouldUseObjectDraftPlane(kind, status)
}
