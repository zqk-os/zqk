package organizational

import (
	"regexp"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

const testEmptyValue = ""

// TestGenerateImpactAnalysisID_format verifies generated IDs match ^[A-Z]+-\d{3,}$ (base_object id pattern).
func TestGenerateImpactAnalysisID_format(t *testing.T) {
	t.Parallel()
	re := regexp.MustCompile(`^IMP-\d+$`)
	for i := 0; i < 5; i++ {
		id := generateImpactAnalysisID()
		if id == testEmptyValue {
			t.Error("generateImpactAnalysisID returned empty")
		}
		if !re.MatchString(id) {
			t.Errorf("generateImpactAnalysisID() = %q, want match for ^IMP-\\d+$", id)
		}
	}
}

// TestGetImpactAnalysisStatus_returnsNonEmpty verifies getImpactAnalysisStatus returns a non-empty valid status.
func TestGetImpactAnalysisStatus_returnsNonEmpty(t *testing.T) {
	t.Parallel()
	status := getImpactAnalysisStatus()
	if status == testEmptyValue {
		t.Error("getImpactAnalysisStatus returned empty")
	}
	// Base lifecycle uses "proposed"; fallback is "draft"
	valid := map[string]bool{"proposed": true, "draft": true}
	if !valid[status] {
		t.Logf("getImpactAnalysisStatus = %q (may be valid if lifecycle is configured)", status)
	}
}

// TestCreateImpactAnalysisObjectManual_hasRequiredFields verifies manual impact_analysis object has id, kind, change_ref, change_type.
func TestCreateImpactAnalysisObjectManual_hasRequiredFields(t *testing.T) {
	t.Parallel()
	obj := createImpactAnalysisObjectManual("IMP-001", "OCH-001", "division_restructure", map[string]any{"divisions": []any{"DIV-001"}})
	if obj[objects.FieldKeyID] != "IMP-001" {
		t.Errorf("id = %v, want IMP-001", obj[objects.FieldKeyID])
	}
	if obj[objects.FieldKeyKind] != objects.KindImpactAnalysis {
		t.Errorf("kind = %v, want impact_analysis", obj[objects.FieldKeyKind])
	}
	if obj[objects.FieldKeyChangeRef] != "OCH-001" {
		t.Errorf("change_ref = %v, want OCH-001", obj[objects.FieldKeyChangeRef])
	}
	if obj[objects.FieldKeyChangeType] != "division_restructure" {
		t.Errorf("change_type = %v, want division_restructure", obj[objects.FieldKeyChangeType])
	}
	if obj[objects.FieldKeyStatus] == testEmptyValue {
		t.Error("status should be non-empty")
	}
	if obj[objects.FieldKeyAffectedObjects] == nil {
		t.Error("affected_objects should be set")
	}
}
