package qapack_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/qapack"
)

func TestQAPackConstantsAndKinds(t *testing.T) {
	if qapack.Name != "qa" {
		t.Fatalf("expected Name to be 'qa', got %q", qapack.Name)
	}
	if qapack.SpecDir != "packs/qa/specs" {
		t.Fatalf("expected SpecDir to be 'packs/qa/specs', got %q", qapack.SpecDir)
	}
	if qapack.LifecycleDir != "packs/qa/lifecycles" {
		t.Fatalf("expected LifecycleDir to be 'packs/qa/lifecycles', got %q", qapack.LifecycleDir)
	}
	if qapack.OmitBuildTag != "zqk_omit_qapack" {
		t.Fatalf("expected OmitBuildTag to be 'zqk_omit_qapack', got %q", qapack.OmitBuildTag)
	}

	kinds := qapack.Kinds()
	if len(kinds) != 10 {
		t.Fatalf("expected 10 kinds, got %d", len(kinds))
	}
	expected := map[string]bool{
		"code_quality_metric":           true,
		"code_reference":                true,
		"maturation_report":             true,
		"process_hygiene_rule":          true,
		"qa_success":                    true,
		"scenario":                      true,
		"test_audit_aggregation_metric": true,
		"test_command_rule":             true,
		"validation_rule":               true,
		"verification_matrix":           true,
	}
	for _, k := range kinds {
		if !expected[k] {
			t.Errorf("unexpected kind %q in qapack", k)
		}
	}
}
