package objects

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

var ontologyStemPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// GenerateObjectSpecKindDraft returns a starter object_specs YAML document for a new kind.
// It loads extendsName.yaml with full inheritance and materializes effective top-level defaults
// (storage_profile, traits, exclude_traits, visibility) so authors see what the loader will merge.
// The leading comment block explains omission vs override; see DATA_CELL_MODEL.md (storage profiles).
func GenerateObjectSpecKindDraft(loader *SpecLoader, ontology, extendsName string) (string, error) {
	ontology = strings.TrimSpace(ontology)
	if ontology == emptyValue {
		return "", errfmt.Errorf("ontology is required")
	}
	if !ontologyStemPattern.MatchString(ontology) {
		return "", errfmt.Errorf("ontology %q must match %s (lowercase stem, e.g. my_kind)", ontology, ontologyStemPattern.String())
	}
	extendsName = strings.TrimSpace(extendsName)
	if extendsName == emptyValue {
		extendsName = "base_object"
	}
	if !ontologyStemPattern.MatchString(extendsName) {
		return "", errfmt.Errorf("extends %q must match %s", extendsName, ontologyStemPattern.String())
	}

	if loader == nil {
		loader = NewSpecLoader("")
	}

	parent, err := loader.LoadSpecWithInheritance(extendsName + ".yaml")
	if err != nil {
		return "", errfmt.Errorf("load extends spec %s.yaml: %w", extendsName, err)
	}
	if parent == nil {
		return "", errfmt.Errorf("nil spec for extends %q", extendsName)
	}

	schemaVersion := parent.SchemaVersion
	if schemaVersion == emptyValue {
		schemaVersion = "2.0.0"
	}
	visibility := strings.TrimSpace(parent.Visibility)
	if visibility == emptyValue {
		visibility = "internal"
	}

	comment := buildObjectSpecKindDraftComment(ontology, extendsName, parent)

	body := struct {
		SchemaVersion  string         `yaml:"schema_version"`
		Ontology       string         `yaml:"ontology"`
		Extends        string         `yaml:"extends"`
		Visibility     string         `yaml:"visibility"`
		StorageProfile string         `yaml:"storage_profile,omitempty"`
		Traits         []string       `yaml:"traits,omitempty"`
		ExcludeTraits  []string       `yaml:"exclude_traits,omitempty"`
		Description    string         `yaml:"description"`
		Fields         map[string]any `yaml:"fields"`
	}{
		SchemaVersion:  schemaVersion,
		Ontology:       ontology,
		Extends:        extendsName,
		Visibility:     visibility,
		StorageProfile: parent.StorageProfile,
		Traits:         NewTraitRegistry().StripRedundantIncludedTraits(parent.ResolvedTraits),
		ExcludeTraits:  append([]string(nil), parent.ExcludeTraits...),
		Description:    "Describe this kind: purpose, lifecycle reference, and operational notes.\n",
		Fields:         map[string]any{},
	}

	yamlBytes, err := yaml.Marshal(&body)
	if err != nil {
		return "", errfmt.Newf("marshal object spec draft").Wrap(err)
	}

	return comment + string(yamlBytes), nil
}

func buildObjectSpecKindDraftComment(ontology, extendsName string, parent *Spec) string {
	var b strings.Builder
	b.WriteString("# Object kind spec (draft) — save as .zqk/specs/objects/")
	b.WriteString(ontology)
	b.WriteString(".yaml\n")
	b.WriteString("#\n")
	b.WriteString("# Inheritance model (object_specs loader):\n")
	b.WriteString("# - `extends` points at the parent kind (YAML stem under object_specs/). Parent fields and traits\n")
	b.WriteString("#   merge into this kind; omit a field block here to inherit parent field definitions.\n")
	b.WriteString("# - Top-level keys you omit may still be resolved at load time from the parent chain (same as\n")
	b.WriteString("#   this draft's effective defaults were copied from the resolved `")
	b.WriteString(extendsName)
	b.WriteString("` spec).\n")
	b.WriteString("# - Override `storage_profile`, `traits`, or `exclude_traits` when this kind needs a different\n")
	b.WriteString("#   data-cell profile than the effective default below (e.g. ")
	b.WriteString(string(datacell.ProfileStream))
	b.WriteString(" for high-volume kinds listed in configs/high_volume_kinds.yaml).\n")
	b.WriteString("# - Remove a copied default key to rely on inheritance; set explicitly to lock a value.\n")
	b.WriteString("#\n")
	if parent != nil && parent.StorageProfile != emptyValue {
		b.WriteString("# Effective defaults below include storage_profile=")
		b.WriteString(parent.StorageProfile)
		b.WriteString(" from the resolved extends chain (see auditable → base_object for CAS-backed kinds).\n")
	} else {
		b.WriteString("# Effective storage_profile is unset on the parent chain; declare storage_profile when you\n")
		b.WriteString("# choose cas_entity, ")
		b.WriteString(string(datacell.ProfileLightFile))
		b.WriteString(", or ")
		b.WriteString(string(datacell.ProfileStream))
		b.WriteString(".\n")
	}
	b.WriteString("#\n\n")
	return b.String()
}
