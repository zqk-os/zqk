package datacell

import (
	"testing"
)

func TestOperationalEnvelopeForKind_AddOverride(t *testing.T) {
	kind := "test_kind"
	kindOperationalEnvelopeOverrides[kind] = map[StorageProfile]operationalEnvelopeKindOverride{
		ProfileStream: {
			AddScan: []string{"new_scan_token"},
		},
	}
	defer delete(kindOperationalEnvelopeOverrides, kind)

	env, ok := OperationalEnvelopeForKind(kind, ProfileStream)
	if !ok {
		t.Fatal("Expected envelope to be found")
	}

	found := false
	for _, s := range env.Scan {
		if s == "new_scan_token" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'new_scan_token' in Scan list, but not found")
	}
}
