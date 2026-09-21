package cas

import (
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

// ParkCriteriaWithoutCategory is true when kind is criteria and category is
// empty after alias remap. Those objects stay on the draft plane; hash CAS
// must not receive them. TRACK: BLI-KERNEL-CRIT-CATEGORY-MINT-001
func ParkCriteriaWithoutCategory(kind string, obj map[string]any) bool {
	return objects.RequireCriteriaCategory(kind, obj) != nil
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
	if ParkCriteriaWithoutCategory(kind, obj) {
		return true
	}
	return ParkObjectWithoutDescription(kind, obj)
}

// RefuseCriteriaCASWithoutCategory is the CAS membrane: hash persist
// (isDraft=false) of criteria without category is fail-closed. Draft-plane
// writes are allowed so create can succeed outside the membrane.
func RefuseCriteriaCASWithoutCategory(kind string, isDraft bool, data []byte) error {
	if isDraft || kind != objects.KindCriteria {
		return nil
	}
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return errfmt.Newf("CAS membrane: unmarshal criteria").Wrap(err)
	}
	objects.CoerceMutationFields(kind, obj)
	return objects.RequireCriteriaCategory(kind, obj)
}

func shouldUseObjectDraftPlane(kind, status string) bool {
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
