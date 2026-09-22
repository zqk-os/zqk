package trait_builders

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type dummyTraitBuilder struct {
	name    string
	version string
	trait   *objects.TraitDefinition
}

func (d *dummyTraitBuilder) Build() *objects.TraitDefinition {
	return d.trait
}

func (d *dummyTraitBuilder) GetVersion() string {
	return d.version
}

func (d *dummyTraitBuilder) GetName() string {
	return d.name
}

func TestVersionedTraitBuilderRegistry(t *testing.T) {
	reg := NewVersionedTraitBuilderRegistry()

	t1 := &objects.TraitDefinition{Name: "listable"}
	t2 := &objects.TraitDefinition{Name: "listable"}
	t3 := &objects.TraitDefinition{Name: "listable"}
	tCustom := &objects.TraitDefinition{Name: "custom_trait"}

	b1 := &dummyTraitBuilder{name: "listable", version: "v1_0_0", trait: t1}
	b2 := &dummyTraitBuilder{name: "listable", version: "v1_2_0", trait: t2}
	b3 := &dummyTraitBuilder{name: "listable", version: "v1_10_0", trait: t3}
	bCustom := &dummyTraitBuilder{name: "custom_trait", version: "custom_ver", trait: tCustom}

	reg.Register(b1)
	reg.Register(b2)
	reg.Register(b3)
	reg.Register(bCustom)

	t.Run("GetBuilder existing", func(t *testing.T) {
		got, err := reg.GetBuilder("listable", "v1_2_0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.GetVersion() != "v1_2_0" {
			t.Fatalf("expected v1_2_0, got %s", got.GetVersion())
		}
	})

	t.Run("GetBuilder missing name", func(t *testing.T) {
		_, err := reg.GetBuilder("nonexistent", "v1_0_0")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("GetBuilder missing version", func(t *testing.T) {
		_, err := reg.GetBuilder("listable", "v9_9_9")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("GetLatestVersion semver comparison", func(t *testing.T) {
		latest, err := reg.GetLatestVersion("listable")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if latest != "v1_10_0" {
			t.Fatalf("expected v1_10_0, got %s", latest)
		}
	})

	t.Run("GetLatestVersion non-semver fallback", func(t *testing.T) {
		latest, err := reg.GetLatestVersion("custom_trait")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if latest != "custom_ver" {
			t.Fatalf("expected custom_ver, got %s", latest)
		}
	})

	t.Run("GetLatestVersion nonexistent", func(t *testing.T) {
		_, err := reg.GetLatestVersion("nonexistent")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("GetAllNames and GetVersions", func(t *testing.T) {
		names := reg.GetAllNames()
		if len(names) < 2 {
			t.Fatalf("expected at least 2 names, got %d", len(names))
		}

		versions := reg.GetVersions("listable")
		if len(versions) != 3 {
			t.Fatalf("expected 3 versions for listable, got %d", len(versions))
		}

		if nilVersions := reg.GetVersions("nonexistent"); nilVersions != nil {
			t.Fatalf("expected nil for nonexistent name versions, got %v", nilVersions)
		}
	})

	t.Run("Global Registry", func(t *testing.T) {
		global := GetGlobalRegistry()
		if global == nil {
			t.Fatal("expected non-nil global registry")
		}
		RegisterBuilder(b1)
		got, err := global.GetBuilder("listable", "v1_0_0")
		if err != nil || got == nil {
			t.Fatalf("expected registered builder in global registry, err: %v", err)
		}
	})
}

func TestBaseTraitBuilder(t *testing.T) {
	builder := NewBaseTraitBuilder("test_trait", "v1_0_0")
	if builder.GetName() != "test_trait" {
		t.Errorf("expected test_trait, got %s", builder.GetName())
	}
	if builder.GetVersion() != "v1_0_0" {
		t.Errorf("expected v1_0_0, got %s", builder.GetVersion())
	}

	builder.SetDescription("Test trait description").
		SetCategory("core").
		SetObjectLevel(true).
		SetFieldLevel(false).
		AddRequires("req_trait").
		AddConflicts("conflict_trait").
		AddIncludes("inc_trait").
		SetConfig(map[string]any{"key": "val"})

	tr := builder.Build()
	if tr.Name != "test_trait" {
		t.Errorf("expected test_trait, got %s", tr.Name)
	}
	if tr.Category != "core" {
		t.Errorf("expected core, got %s", tr.Category)
	}
	if !tr.ObjectLevel || tr.FieldLevel {
		t.Errorf("unexpected level flags: object=%v field=%v", tr.ObjectLevel, tr.FieldLevel)
	}
	if len(tr.Requires) != 1 || tr.Requires[0] != "req_trait" {
		t.Fatalf("unexpected requires: %#v", tr.Requires)
	}
	if len(tr.Conflicts) != 1 || tr.Conflicts[0] != "conflict_trait" {
		t.Fatalf("unexpected conflicts: %#v", tr.Conflicts)
	}
	if len(tr.Includes) != 1 || tr.Includes[0] != "inc_trait" {
		t.Fatalf("unexpected includes: %#v", tr.Includes)
	}
	if tr.Config["key"] != "val" {
		t.Fatalf("unexpected config: %#v", tr.Config)
	}
}

func TestTraitGenerator(t *testing.T) {
	tempDir := t.TempDir()
	gen := NewTraitGenerator(tempDir)

	if err := gen.EnsureOutputDir(); err != nil {
		t.Fatalf("unexpected EnsureOutputDir error: %v", err)
	}

	dummyT := &objects.TraitDefinition{
		Name:        "gen_trait",
		Description: "generated trait",
	}
	b := &dummyTraitBuilder{name: "gen_trait", version: "v1_0_0", trait: dummyT}
	gen.registry.Register(b)

	t.Run("GenerateTrait", func(t *testing.T) {
		err := gen.GenerateTrait("gen_trait", "v1_0_0")
		if err != nil {
			t.Fatalf("unexpected GenerateTrait error: %v", err)
		}
		path := filepath.Join(tempDir, "gen_trait.yaml")
		content, err := fileutil.ReadFile(path)
		if err != nil {
			t.Fatalf("expected file to exist: %v", err)
		}
		if len(content) == 0 {
			t.Fatal("expected non-empty trait file")
		}
	})

	t.Run("GenerateLatestTrait", func(t *testing.T) {
		err := gen.GenerateLatestTrait("gen_trait")
		if err != nil {
			t.Fatalf("unexpected GenerateLatestTrait error: %v", err)
		}
	})

	t.Run("GenerateAllTraits", func(t *testing.T) {
		err := gen.GenerateAllTraits()
		if err != nil {
			t.Fatalf("unexpected GenerateAllTraits error: %v", err)
		}
	})
}

func TestTraitCodegenHelpers(t *testing.T) {
	t.Run("toCamelCase", func(t *testing.T) {
		cases := map[string]string{
			"listable_trait": "ListableTrait",
			"single":         "Single",
			"":               "",
		}
		for in, want := range cases {
			if got := toCamelCase(in); got != want {
				t.Errorf("toCamelCase(%q) = %q, want %q", in, got, want)
			}
		}
	})

	t.Run("formatValue", func(t *testing.T) {
		input := map[string]any{
			"str":     "val",
			"int":     1,
			"uint":    uint(2),
			"float":   1.5,
			"bool":    true,
			"nilval":  nil,
			"list":    []any{"item", 10},
			"unknown": complex(1, 2),
		}
		res := formatValue(input, 1)
		if res == "" {
			t.Fatal("expected non-empty formatValue output")
		}
	})

	t.Run("generateBuilderCode", func(t *testing.T) {
		code, err := generateBuilderCode(
			"listable",
			"listable trait",
			"core",
			true,
			false,
			[]string{"req1"},
			[]string{"conf1"},
			[]string{"inc1"},
			map[string]any{"obj_k": "v"},
			map[string]any{"fld_k": "v"},
			nil,
			"listable",
			"v1_0_0",
			"test_pkg",
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code == "" {
			t.Fatal("expected non-empty code")
		}
	})

	t.Run("GenerateBuilderFromYAML", func(t *testing.T) {
		tempDir := t.TempDir()
		outDir := filepath.Join(tempDir, "pkg", "specbuilder", "trait_builders")
		if err := fileutil.MkdirAll(outDir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create outDir: %v", err)
		}

		yamlPath := filepath.Join(tempDir, "sample_trait.yaml")
		_ = fileutil.WriteFile(yamlPath, []byte("name: sample\ndescription: sample trait\nstatus: active\nrequires:\n  - req1\n"), paths.FilePerm644)

		err := GenerateBuilderFromYAML(yamlPath, outDir)
		if err != nil {
			t.Fatalf("unexpected GenerateBuilderFromYAML error: %v", err)
		}

		// Inactive trait skipped
		inactivePath := filepath.Join(tempDir, "inactive_trait.yaml")
		_ = fileutil.WriteFile(inactivePath, []byte("name: inactive\nstatus: inactive\n"), paths.FilePerm644)
		if err := GenerateBuilderFromYAML(inactivePath, outDir); err == nil {
			t.Error("expected error for inactive trait")
		}
	})
}
