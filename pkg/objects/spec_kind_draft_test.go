package objects

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGenerateObjectSpecKindDraft_InheritedDefaults(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	out, err := GenerateObjectSpecKindDraft(loader, "my_kind", "base_object")
	if err != nil {
		t.Fatalf("GenerateObjectSpecKindDraft: %v", err)
	}
	if !strings.Contains(out, "storage_profile: cas_entity") {
		t.Fatalf("expected inherited cas_entity from auditable chain; got:\n%s", out)
	}
	if !strings.Contains(out, "extends: base_object") {
		t.Fatal("missing extends")
	}
	if !strings.Contains(out, "Inheritance model") {
		t.Fatal("expected inheritance comment block")
	}

	streamOut, err := GenerateObjectSpecKindDraft(loader, "hv_kind", "base_metric")
	if err != nil {
		t.Fatalf("GenerateObjectSpecKindDraft base_metric: %v", err)
	}
	if !strings.Contains(streamOut, "storage_profile: stream") {
		t.Fatalf("expected stream profile from base_metric chain; got:\n%s", streamOut)
	}
}

func TestGenerateObjectSpecKindDraft_StripsIncludedTraitGroups(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	out, err := GenerateObjectSpecKindDraft(loader, "my_kind", "base_object")
	if err != nil {
		t.Fatalf("GenerateObjectSpecKindDraft: %v", err)
	}
	var yamlBody strings.Builder
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		yamlBody.WriteString(line)
		yamlBody.WriteByte('\n')
	}
	var parsed struct {
		Traits []string `yaml:"traits"`
	}
	if err := yaml.Unmarshal([]byte(yamlBody.String()), &parsed); err != nil {
		t.Fatalf("unmarshal draft yaml: %v\n%s", err, yamlBody.String())
	}
	hasBase := false
	for _, tr := range parsed.Traits {
		if tr == "base_auditable_traits" {
			t.Fatalf("draft restated included group base_auditable_traits: %v", parsed.Traits)
		}
		if tr == "base_object_traits" {
			hasBase = true
		}
	}
	if !hasBase {
		t.Fatalf("expected base_object_traits in draft traits, got %v", parsed.Traits)
	}
}
