package profile_builders

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type dummyProfileBuilder struct {
	name    string
	version string
	profile *config.UnifiedProfile
}

func (d *dummyProfileBuilder) Build() *config.UnifiedProfile {
	return d.profile
}

func (d *dummyProfileBuilder) GetVersion() string {
	return d.version
}

func (d *dummyProfileBuilder) GetName() string {
	return d.name
}

func TestVersionedProfileBuilderRegistry(t *testing.T) {
	reg := NewVersionedProfileBuilderRegistry()

	p1 := &config.UnifiedProfile{Kind: "profile"}
	p2 := &config.UnifiedProfile{Kind: "profile"}
	p3 := &config.UnifiedProfile{Kind: "profile"}
	pCustom := &config.UnifiedProfile{Kind: "profile"}

	b1 := &dummyProfileBuilder{name: "base_router", version: "v1_0_0", profile: p1}
	b2 := &dummyProfileBuilder{name: "base_router", version: "v1_2_0", profile: p2}
	b3 := &dummyProfileBuilder{name: "base_router", version: "v1_10_0", profile: p3}
	bCustom := &dummyProfileBuilder{name: "custom_router", version: "custom_ver", profile: pCustom}

	reg.Register(b1)
	reg.Register(b2)
	reg.Register(b3)
	reg.Register(bCustom)

	t.Run("GetBuilder existing", func(t *testing.T) {
		got, err := reg.GetBuilder("base_router", "v1_2_0")
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
		_, err := reg.GetBuilder("base_router", "v9_9_9")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("GetLatestVersion semver comparison", func(t *testing.T) {
		latest, err := reg.GetLatestVersion("base_router")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if latest != "v1_10_0" {
			t.Fatalf("expected v1_10_0, got %s", latest)
		}
	})

	t.Run("GetLatestVersion non-semver fallback", func(t *testing.T) {
		latest, err := reg.GetLatestVersion("custom_router")
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

		versions := reg.GetVersions("base_router")
		if len(versions) != 3 {
			t.Fatalf("expected 3 versions for base_router, got %d", len(versions))
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
		got, err := global.GetBuilder("base_router", "v1_0_0")
		if err != nil || got == nil {
			t.Fatalf("expected registered builder in global registry, err: %v", err)
		}
	})
}

func TestBaseProfileBuilder(t *testing.T) {
	builder := NewBaseProfileBuilder("test_profile", "v1_0_0")
	if builder.GetName() != "test_profile" {
		t.Errorf("expected test_profile, got %s", builder.GetName())
	}
	if builder.GetVersion() != "v1_0_0" {
		t.Errorf("expected v1_0_0, got %s", builder.GetVersion())
	}

	builder.SetSchemaVersion("1.0.0").
		SetKind("profile").
		SetType(config.ProfileTypeTransceiverRouter).
		SetMetadata(config.ProfileMetadata{
			Name:        "test_profile",
			Description: "desc",
		}).
		SetSpec(map[string]any{
			"max_workers": 10,
		})

	p := builder.Build()
	if p.SchemaVersion != "1.0.0" {
		t.Errorf("expected 1.0.0, got %s", p.SchemaVersion)
	}
	if p.Kind != "profile" {
		t.Errorf("expected profile, got %s", p.Kind)
	}
	if p.Type != config.ProfileTypeTransceiverRouter {
		t.Errorf("expected transceiver_router, got %s", p.Type)
	}
	if p.Metadata.Name != "test_profile" {
		t.Errorf("expected test_profile, got %s", p.Metadata.Name)
	}
	if p.Spec["max_workers"] != 10 {
		t.Fatalf("unexpected spec: %#v", p.Spec)
	}
}

func TestProfileGenerator(t *testing.T) {
	tempDir := t.TempDir()
	gen := NewProfileGenerator(tempDir)

	if err := gen.EnsureOutputDir(); err != nil {
		t.Fatalf("unexpected EnsureOutputDir error: %v", err)
	}

	dummyP := &config.UnifiedProfile{
		SchemaVersion: "1.0.0",
		Kind:          "profile",
		Metadata: config.ProfileMetadata{
			Name: "gen_test",
		},
	}
	b := &dummyProfileBuilder{name: "gen_test", version: "v1_0_0", profile: dummyP}
	gen.registry.Register(b)

	t.Run("GenerateProfile", func(t *testing.T) {
		err := gen.GenerateProfile("gen_test", "v1_0_0")
		if err != nil {
			t.Fatalf("unexpected GenerateProfile error: %v", err)
		}
		path := filepath.Join(tempDir, "gen_test.yaml")
		content, err := fileutil.ReadFile(path)
		if err != nil {
			t.Fatalf("expected file to exist: %v", err)
		}
		if len(content) == 0 {
			t.Fatal("expected non-empty profile file")
		}
	})

	t.Run("GenerateLatestProfile", func(t *testing.T) {
		err := gen.GenerateLatestProfile("gen_test")
		if err != nil {
			t.Fatalf("unexpected GenerateLatestProfile error: %v", err)
		}
	})

	t.Run("GenerateAllProfiles", func(t *testing.T) {
		err := gen.GenerateAllProfiles()
		if err != nil {
			t.Fatalf("unexpected GenerateAllProfiles error: %v", err)
		}
	})
}

func TestProfileCodegenHelpers(t *testing.T) {
	t.Run("toCamelCase", func(t *testing.T) {
		cases := map[string]string{
			"base_router": "BaseRouter",
			"single":      "Single",
			"":            "",
		}
		for in, want := range cases {
			if got := toCamelCase(in); got != want {
				t.Errorf("toCamelCase(%q) = %q, want %q", in, got, want)
			}
		}
	})

	t.Run("formatValueWithFieldKeys", func(t *testing.T) {
		wireToConst := map[string]string{
			"enabled": "FieldKeyEnabled",
		}
		input := map[string]any{
			"enabled": true,
			"workers": 5,
			"uintval": uint(10),
			"rate":    1.5,
			"desc":    "router",
			"nilval":  nil,
			"list":    []any{"sub", 2},
		}
		res := formatValueWithFieldKeys(input, 1, wireToConst)
		if res == "" {
			t.Fatal("expected non-empty formatValueWithFieldKeys")
		}
	})

	t.Run("specUsesFieldKeyLiterals", func(t *testing.T) {
		wireToConst := map[string]string{
			"enabled": "FieldKeyEnabled",
		}
		if !specUsesFieldKeyLiterals(map[string]any{"enabled": true}, wireToConst) {
			t.Error("expected true for matching field key")
		}
		if !specUsesFieldKeyLiterals([]any{map[string]any{"enabled": true}}, wireToConst) {
			t.Error("expected true for nested matching field key")
		}
		if specUsesFieldKeyLiterals(map[string]any{"other": 1}, wireToConst) {
			t.Error("expected false for non-matching field key")
		}
	})

	t.Run("generateBuilderCode", func(t *testing.T) {
		profile := &config.UnifiedProfile{
			SchemaVersion: "1.0.0",
			Kind:          "profile",
			Type:          config.ProfileTypeTransceiverRouter,
			Metadata: config.ProfileMetadata{
				Name:        "base_router",
				Description: "test router",
			},
			Spec: map[string]any{
				"enabled": true,
			},
		}
		code, err := generateBuilderCode(profile, "base_router", "v1_0_0", "test_pkg", map[string]string{"enabled": "FieldKeyEnabled"}, "github.com/zqk-os/zqk", "profile_builders")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code == "" {
			t.Fatal("expected non-empty code")
		}
	})

	t.Run("GenerateBuilderFromYAML", func(t *testing.T) {
		tempDir := t.TempDir()
		outDir := filepath.Join(tempDir, "pkg", "specbuilder", "profile_builders")
		if err := fileutil.MkdirAll(outDir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create outDir: %v", err)
		}
		_ = fileutil.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module github.com/zqk-os/zqk\n"), paths.FilePerm644)
		objectsDir := filepath.Join(tempDir, "pkg", "objects")
		if err := fileutil.MkdirAll(objectsDir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create objectsDir: %v", err)
		}
		_ = fileutil.WriteFile(filepath.Join(objectsDir, "field_keys.go"), []byte("package objects\n\nconst (\n\tFieldKeyEnabled = \"enabled\"\n)\n"), paths.FilePerm644)

		yamlPath := filepath.Join(tempDir, "sample_profile.yaml")
		_ = fileutil.WriteFile(yamlPath, []byte("schema_version: \"1.0.0\"\nkind: profile\ntype: transceiver_router\nmetadata:\n  name: sample\nspec:\n  workers: 4\n"), paths.FilePerm644)

		err := GenerateBuilderFromYAML(yamlPath, outDir)
		if err != nil {
			t.Fatalf("unexpected GenerateBuilderFromYAML error: %v", err)
		}
	})
}
