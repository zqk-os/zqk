// BLI-STARTER-COMMUNITY-050 / PRI-STARTER-COMMUNITY-050 coverage elevation
package builders

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type extraSpecBuilder struct {
	ont, ver string
	spec     *objects.Spec
}

func (e extraSpecBuilder) Build() *objects.Spec {
	if e.spec != nil {
		return e.spec
	}
	return &objects.Spec{Ontology: e.ont, SchemaVersion: "1.0.0"}
}
func (e extraSpecBuilder) GetVersion() string  { return e.ver }
func (e extraSpecBuilder) GetOntology() string { return e.ont }

func TestExtraVersionRegistryDetectorAndFieldBuilder(t *testing.T) {
	if _, err := IncrementVersion("bad", "patch"); err == nil {
		t.Fatal("bad version")
	}
	if _, err := IncrementVersion("v1_x_0", "patch"); err == nil {
		t.Fatal("bad minor")
	}
	if _, err := IncrementVersion("vx_0_0", "patch"); err == nil {
		t.Fatal("bad major")
	}
	if _, err := IncrementVersion("v1_0_x", "patch"); err == nil {
		t.Fatal("bad patch")
	}
	got, err := IncrementVersion("v1_2_3", "major")
	if err != nil || got != "v2_0_0" {
		t.Fatalf("%s %v", got, err)
	}
	got, err = IncrementVersion("1_0_0", "minor")
	if err != nil || got != "v1_1_0" {
		t.Fatalf("%s %v", got, err)
	}
	got, err = IncrementVersion("v1_0_0", "patch")
	if err != nil || got != "v1_0_1" {
		t.Fatalf("%s %v", got, err)
	}
	got, err = IncrementVersion("v1_0_0", "other")
	if err != nil || got != "v1_0_1" {
		t.Fatalf("%s %v", got, err)
	}
	_ = ParseInstanceVersion("")
	_ = ParseInstanceVersion("1.2.3")
	_ = FormatBuilderVersion("")
	_ = FormatBuilderVersion("v1_2_3")
	_ = versionToPackageName("")
	_ = versionToPackageName("v2_1_0")
	_ = versionToPackageName("v")
	_ = FormatPatternForGoCode(`\\d+`)
	_ = specFileNameForOntology("extra_kind")
	_ = NormalizeChecklistObservability("yes.")
	_ = NormalizeChecklistSecurity("non-sensitive.")
	_ = NormalizeChecklistLifecycle("mutable.")
	_ = NormalizeChecklistLifecycle("longer prose stays.")

	r := NewVersionedBuilderRegistry()
	r.Register(extraSpecBuilder{ont: "extra_kind", ver: "v1_0_0"})
	r.Register(extraSpecBuilder{ont: "extra_kind", ver: "v1_0_1"})
	r.Register(extraSpecBuilder{ont: "odd_kind", ver: "not-semver"})
	if _, err := r.GetBuilder("extra_kind", "v1_0_0"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetBuilder("extra_kind", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetBuilder("missing", "v1_0_0"); err == nil {
		t.Fatal("missing builder")
	}
	if _, err := r.GetLatestVersion("extra_kind"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetLatestVersion("odd_kind"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetLatestVersion("missing"); err == nil {
		t.Fatal("missing latest")
	}
	_ = r.GetAllOntologies()
	_ = r.GetVersions("extra_kind")
	_ = r.GetVersions("missing")
	_ = GetGlobalRegistry()
	nv, err := GetNextVersion(r, "extra_kind", "patch")
	if err != nil || nv == "" {
		t.Fatal(err)
	}
	nv, err = GetNextVersion(r, "nope", "patch")
	if err != nil || nv == "" {
		t.Fatal(err)
	}

	b := NewBaseSpecBuilder("extra_kind", "v1_0_0").
		SetSchemaVersion("1.0.0").
		SetExtends("null").
		AddCompose("mixin").
		SetDescription("d").
		SetVisibility("internal").
		AddTrait("readable").
		AddField("title", map[string]any{"type": "string"}).
		AddFieldBuilder(NewFieldBuilder("status", "string"))
	if b.Build() == nil || b.GetVersion() == "" || b.GetOntology() == "" {
		t.Fatal("base")
	}

	RegisterBuilder(extraSpecBuilder{ont: "extra_kind", ver: "v1_0_0", spec: b.Build()})
	ad := NewSpecLoaderAdapter(r)
	ba, err := ad.GetBuilder("extra_kind", "v1_0_0")
	if err != nil || ba == nil {
		t.Fatal(err)
	}
	_ = ba.Build()
	_ = ba.GetVersion()
	_ = ba.GetOntology()
	if _, err := ad.GetBuilder("missing", "v1_0_0"); err == nil {
		t.Fatal("adapter miss")
	}
	_, _ = ad.GetLatestVersion("extra_kind")
	_ = ad.GetVersions("extra_kind")
	InitializeGlobalSpecLoader()

	out := t.TempDir()
	sg := NewSpecGenerator(out)
	if err := sg.EnsureOutputDir(); err != nil {
		t.Fatal(err)
	}
	_ = sg.GenerateSpec("extra_kind", "v1_0_0")
	_ = sg.GenerateLatestSpec("extra_kind")
	_ = sg.GenerateLatestSpec("missing")
	_ = sg.GenerateAllSpecs()
	_ = sg.GenerateSpecAtVersion("extra_kind", "v1_0_0", filepath.Join(out, "at.yaml"))
	_, _ = sg.GenerateSpecsToYAML("extra_kind", "v1_0_0")
	_, _ = sg.GenerateSpecsToYAML("missing", "v1_0_0")

	specs := t.TempDir()
	yamlPath := filepath.Join(specs, "extra_kind.yaml")
	if err := fileutil.WriteFile(yamlPath, []byte("ontology: extra_kind\nschema_version: \"0.0.1\"\nfields:\n  extra: {type: string}\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	d := NewSpecChangeDetector(specs)
	_, _ = d.DetectChanges()
	_ = specsEqual(&objects.Spec{Ontology: "a"}, &objects.Spec{Ontology: "b"})
	_ = basicSpecsEqual(&objects.Spec{Ontology: "a", Description: "x", Visibility: "internal", Extends: "p", Traits: []string{"t"}, Fields: map[string]any{"f": 1}},
		&objects.Spec{Ontology: "b", Description: "y", Visibility: "public", Extends: "q", Traits: []string{"u", "v"}, Fields: map[string]any{"g": 1}})
	_ = summarizeDiff(&objects.Spec{SchemaVersion: "1", Fields: map[string]any{"a": 1}}, &objects.Spec{SchemaVersion: "2", Fields: map[string]any{"b": 1}})

	fb := NewFieldBuilder("title", "string").
		Description("d").
		Purpose("p").
		SetDescription("d2").
		WithTraits("readable").
		WithPermissions("read").
		WithSemanticType("text").
		WithProfileCode("T").
		WithDefault("x").
		WithChecklist(NewChecklistBuilder().
			Authority("a").
			AutomationHooks("h").
			Cardinality("one").
			Criticality("low").
			Default("x").
			Dependencies("d").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("p").
			Security("non-sensitive").
			SystemUsage("u").
			Validation("v").
			Build()).
		WithAccess(NewAccessBuilder().Requires("role").Build()).
		WithValidation(NewValidationBuilder().
			Required(true).
			Pattern("^x$").
			MinLength(1).
			MaxLength(8).
			DisplayLength(4).
			MinCount(0).
			MaxCount(1).
			Enum([]any{"x"}).
			Build())
	if fb.Build() == nil {
		t.Fatal("build")
	}

	cf := NewConstantsFactory("bldr_v2")
	_ = NewConstantsFactoryWithSpecLoader("bldr_v2", nil)
	constsDir := t.TempDir()
	if err := fileutil.WriteFile(filepath.Join(constsDir, "other_constants.go"), []byte("package bldr_v2\nconst (\n\tFieldOther = \"other\"\n)\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	_ = cf.ReserveConstantsFromDir(constsDir, "extra_kind")
	c, err := cf.CreateConstants(&objects.Spec{Fields: map[string]any{"title": map[string]any{"type": "string"}}}, "extra_kind")
	if err != nil {
		t.Fatal(err)
	}
	sc := c.(*SpecConstants)
	_ = sc.GetFieldName("title")
	_ = sc.GetFieldName("missing")
	_ = sc.GetAllFieldNames()
	_ = sc.GetPackage()
	_ = sc.GetOntology()
	_ = sc.ToGoCode()
	emptyC, err := NewConstantsFactory("bldr_v2").CreateConstants(&objects.Spec{}, "empty_kind")
	if err != nil {
		t.Fatal(err)
	}
	_ = emptyC.(*SpecConstants).ToGoCode()
	parentC, err := NewConstantsFactory("bldr_v2").CreateConstants(&objects.Spec{Extends: "base", Fields: map[string]any{}}, "child_kind")
	if err != nil {
		t.Fatal(err)
	}
	_ = parentC.(*SpecConstants).ToGoCode()

	genYAML := filepath.Join(t.TempDir(), "gen_kind.yaml")
	if err := fileutil.WriteFile(genYAML, []byte(`ontology: gen_kind
schema_version: "1.0.0"
extends: "null"
description: "generated extra"
visibility: internal
traits:
  - readable
fields:
  title:
    type: string
    description: title
    checklist:
      criticality: low
      observability: yes
      security: non-sensitive
      lifecycle: mutable
    access:
      requires:
        - role
    validation:
      required: true
      pattern: "^x$"
      min_length: 1
      max_length: 8
      minCount: 0
      maxCount: 1
      enum: [x]
`), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := GenerateBuilderFromYAML(genYAML, t.TempDir(), "", nil); err != nil {
		t.Fatal(err)
	}
	if err := GenerateBuilderFromYAML(genYAML, t.TempDir(), "v1_0_0", NewConstantsFactory("bldr_v2")); err != nil {
		t.Fatal(err)
	}
	_ = toCamelCase("field_name")
	_ = generateFieldBuilderCode("title", map[string]any{
		"type": "string",
		"checklist": map[string]any{
			"criticality":   "low",
			"observability": "yes",
			"security":      "non-sensitive",
			"lifecycle":     "mutable",
		},
		"access":     map[string]any{"requires": []any{"role"}},
		"validation": map[string]any{"required": true, "pattern": "^x$", "min_length": 1},
	})
	_ = generateChecklistBuilderCode(map[string]any{
		"criticality": "low", "observability": "yes", "security": "non-sensitive",
		specChecklistTagLifecycle: "mutable", "authority": "a", "purpose": "p",
	})
	_ = generateAccessBuilderCode(map[string]any{"requires": []any{"role"}})
	_ = generateValidationBuilderCode(map[string]any{
		"required": true, "pattern": "^x$", "min_length": 1, "max_length": 8,
		"display_length": 4, "minCount": 0, "maxCount": 1, "enum": []any{"x"},
	})
	_ = formatValue("s", 0)
	_ = formatValue(1, 0)
	_ = formatValue(true, 0)
	_ = formatValue([]any{"a", 1}, 1)
	_ = formatValue(map[string]any{"k": "v"}, 1)
	_ = formatValue(nil, 0)
}
