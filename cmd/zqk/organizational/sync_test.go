package organizational

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/objects"
)

// Valid org-structure snippet for tests (minimal YAML array).
const sampleOrgSyncYAML = `
- id: ORG-001
  kind: organization
  namespace_id: domain:organizational
  schema_version: "` + objects.DefaultSchemaVersion + `"
  title: Example Corp
  status: active
  domain: custom
  spec_interpreter: default
  spec_context_broker: default
  organization_name: Example Corp
  division_refs: []
- id: DIV-001
  kind: division
  namespace_id: domain:organizational
  schema_version: "` + objects.DefaultSchemaVersion + `"
  title: Engineering
  status: active
  domain: custom
  spec_interpreter: default
  spec_context_broker: default
  division_name: Engineering
  parent_division_ref: null
  team_refs: []
`

// TestOrgSyncAllowedKinds verifies allowed kinds for organizational sync.
func TestOrgSyncAllowedKinds(t *testing.T) {
	t.Parallel()
	want := map[string]bool{
		"organization": true, "division": true, "department": true,
		"team": true, "partnership": true, "organizational_change": true,
	}
	for k, v := range want {
		if !orgSyncAllowedKinds[k] {
			t.Errorf("orgSyncAllowedKinds[%q] = false, want true", k)
		}
		_ = v
	}
	if len(orgSyncAllowedKinds) != len(want) {
		t.Errorf("orgSyncAllowedKinds has %d entries, want %d", len(orgSyncAllowedKinds), len(want))
	}
}

// TestUnmarshalOrgSync_validYAML parses sample YAML and checks object count and kinds.
func TestUnmarshalOrgSync_validYAML(t *testing.T) {
	t.Parallel()
	objSlice, err := unmarshalOrgSync([]byte(sampleOrgSyncYAML), storage.ExportFormatYAML)
	if err != nil {
		t.Fatalf("unmarshalOrgSync: %v", err)
	}
	if len(objSlice) < 2 {
		t.Errorf("expected at least 2 objects, got %d", len(objSlice))
	}
	kind0, _ := objSlice[0][objects.FieldKeyKind].(string)
	if kind0 != "organization" {
		t.Errorf("first object kind = %q, want organization", kind0)
	}
	kind1, _ := objSlice[1][objects.FieldKeyKind].(string)
	if kind1 != "division" {
		t.Errorf("second object kind = %q, want division", kind1)
	}
}

// TestMarshalOrgSync_roundTrip marshals then unmarshals and checks object count.
func TestMarshalOrgSync_roundTrip(t *testing.T) {
	t.Parallel()
	objSlice, err := unmarshalOrgSync([]byte(sampleOrgSyncYAML), storage.ExportFormatYAML)
	if err != nil {
		t.Fatalf("unmarshalOrgSync: %v", err)
	}
	data, err := marshalOrgSync(objSlice, storage.ExportFormatYAML)
	if err != nil {
		t.Fatalf("marshalOrgSync: %v", err)
	}
	round, err := unmarshalOrgSync(data, storage.ExportFormatYAML)
	if err != nil {
		t.Fatalf("unmarshalOrgSync round: %v", err)
	}
	if len(round) != len(objSlice) {
		t.Errorf("round-trip object count: got %d, want %d", len(round), len(objSlice))
	}
}

// TestAllowedKindsList_nonEmpty verifies allowedKindsList returns a non-empty string.
func TestAllowedKindsList_nonEmpty(t *testing.T) {
	t.Parallel()
	list := allowedKindsList()
	if list == emptyValue {
		t.Error("allowedKindsList returned empty")
	}
	if !strings.Contains(list, "organization") {
		t.Errorf("allowedKindsList should contain organization, got %q", list)
	}
}
