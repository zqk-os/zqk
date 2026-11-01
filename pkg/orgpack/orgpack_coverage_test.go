package orgpack_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/orgpack"
)

func TestOrgPackConstantsAndKinds(t *testing.T) {
	if orgpack.Name != "org" {
		t.Fatalf("expected Name to be 'org', got %q", orgpack.Name)
	}
	if orgpack.SpecDir != "packs/org/specs" {
		t.Fatalf("expected SpecDir to be 'packs/org/specs', got %q", orgpack.SpecDir)
	}
	if orgpack.LifecycleDir != "packs/org/lifecycles" {
		t.Fatalf("expected LifecycleDir to be 'packs/org/lifecycles', got %q", orgpack.LifecycleDir)
	}
	if orgpack.OmitBuildTag != "zqk_omit_orgpack" {
		t.Fatalf("expected OmitBuildTag to be 'zqk_omit_orgpack', got %q", orgpack.OmitBuildTag)
	}

	kinds := orgpack.Kinds()
	if len(kinds) != 9 {
		t.Fatalf("expected 9 kinds, got %d", len(kinds))
	}
	expected := map[string]bool{
		"corporate_initiative":  true,
		"department":            true,
		"division":              true,
		"organization":          true,
		"organizational_change": true,
		"partnership":           true,
		"stakeholder_profile":   true,
		"team":                  true,
		"team_configuration":    true,
	}
	for _, k := range kinds {
		if !expected[k] {
			t.Errorf("unexpected kind %q in orgpack", k)
		}
	}
}
