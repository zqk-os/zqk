package routing_builders

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type dummyRoutingRuleBuilder struct {
	fileName string
	version  string
	data     []byte
	err      error
}

func (d *dummyRoutingRuleBuilder) Build() ([]byte, error) {
	return d.data, d.err
}

func (d *dummyRoutingRuleBuilder) GetVersion() string {
	return d.version
}

func (d *dummyRoutingRuleBuilder) GetFileName() string {
	return d.fileName
}

func TestVersionedRoutingRuleBuilderRegistry(t *testing.T) {
	reg := NewVersionedRoutingRuleBuilderRegistry()

	b1 := &dummyRoutingRuleBuilder{fileName: "default_rules", version: "v1_0_0", data: []byte("rules1")}
	b2 := &dummyRoutingRuleBuilder{fileName: "default_rules", version: "v1_2_0", data: []byte("rules2")}
	b3 := &dummyRoutingRuleBuilder{fileName: "default_rules", version: "v1_10_0", data: []byte("rules3")}
	bCustom := &dummyRoutingRuleBuilder{fileName: "custom_rules", version: "custom_ver", data: []byte("custom")}

	reg.Register(b1)
	reg.Register(b2)
	reg.Register(b3)
	reg.Register(bCustom)

	t.Run("GetBuilder existing", func(t *testing.T) {
		got, err := reg.GetBuilder("default_rules", "v1_2_0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.GetVersion() != "v1_2_0" {
			t.Fatalf("expected v1_2_0, got %s", got.GetVersion())
		}
	})

	t.Run("GetBuilder missing fileName", func(t *testing.T) {
		_, err := reg.GetBuilder("nonexistent", "v1_0_0")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("GetBuilder missing version", func(t *testing.T) {
		_, err := reg.GetBuilder("default_rules", "v9_9_9")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("GetLatestVersion semver comparison", func(t *testing.T) {
		latest, err := reg.GetLatestVersion("default_rules")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if latest != "v1_10_0" {
			t.Fatalf("expected v1_10_0, got %s", latest)
		}
	})

	t.Run("GetLatestVersion non-semver fallback", func(t *testing.T) {
		latest, err := reg.GetLatestVersion("custom_rules")
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

	t.Run("GetAllFileNames and GetVersions", func(t *testing.T) {
		files := reg.GetAllFileNames()
		if len(files) < 2 {
			t.Fatalf("expected at least 2 files, got %d", len(files))
		}

		versions := reg.GetVersions("default_rules")
		if len(versions) != 3 {
			t.Fatalf("expected 3 versions for default_rules, got %d", len(versions))
		}

		if nilVersions := reg.GetVersions("nonexistent"); nilVersions != nil {
			t.Fatalf("expected nil for nonexistent file versions, got %v", nilVersions)
		}
	})

	t.Run("Global Registry", func(t *testing.T) {
		global := GetGlobalRegistry()
		if global == nil {
			t.Fatal("expected non-nil global registry")
		}
		RegisterBuilder(b1)
		got, err := global.GetBuilder("default_rules", "v1_0_0")
		if err != nil || got == nil {
			t.Fatalf("expected registered builder in global registry, err: %v", err)
		}
	})
}

func TestBaseRoutingRuleBuilder(t *testing.T) {
	builder := NewBaseRoutingRuleBuilder("test_rules", "v1_0_0")
	if builder.GetFileName() != "test_rules" {
		t.Errorf("expected test_rules, got %s", builder.GetFileName())
	}
	if builder.GetVersion() != "v1_0_0" {
		t.Errorf("expected v1_0_0, got %s", builder.GetVersion())
	}

	builder.AddRule(map[string]any{"source": "cli", "target": "router"})
	data, err := builder.Build()
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty YAML output")
	}
}

func TestRoutingRuleGenerator(t *testing.T) {
	tempDir := t.TempDir()
	gen := NewRoutingRuleGenerator(tempDir)

	if err := gen.EnsureOutputDir(); err != nil {
		t.Fatalf("unexpected EnsureOutputDir error: %v", err)
	}

	b := &dummyRoutingRuleBuilder{fileName: "gen_rules", version: "v1_0_0", data: []byte("source: a\ntarget: b\n")}
	gen.registry.Register(b)

	t.Run("GenerateRoutingRules", func(t *testing.T) {
		err := gen.GenerateRoutingRules("gen_rules", "v1_0_0")
		if err != nil {
			t.Fatalf("unexpected GenerateRoutingRules error: %v", err)
		}
		path := filepath.Join(tempDir, "gen_rules.yaml")
		content, err := fileutil.ReadFile(path)
		if err != nil {
			t.Fatalf("expected file to exist: %v", err)
		}
		if string(content) != "source: a\ntarget: b\n" {
			t.Fatalf("unexpected content: %s", string(content))
		}
	})

	t.Run("GenerateLatestRoutingRules", func(t *testing.T) {
		err := gen.GenerateLatestRoutingRules("gen_rules")
		if err != nil {
			t.Fatalf("unexpected GenerateLatestRoutingRules error: %v", err)
		}
	})

	t.Run("GenerateAllRoutingRules", func(t *testing.T) {
		err := gen.GenerateAllRoutingRules()
		if err != nil {
			t.Fatalf("unexpected GenerateAllRoutingRules error: %v", err)
		}
	})
}

func TestRoutingCodegenHelpers(t *testing.T) {
	t.Run("toCamelCase", func(t *testing.T) {
		cases := map[string]string{
			"default_rules": "DefaultRules",
			"single":        "Single",
			"":              "",
		}
		for in, want := range cases {
			if got := toCamelCase(in); got != want {
				t.Errorf("toCamelCase(%q) = %q, want %q", in, got, want)
			}
		}
	})

	t.Run("formatValueWithFieldKeys", func(t *testing.T) {
		wireToConst := map[string]string{
			"status": "FieldKeyStatus",
		}
		input := map[string]any{
			"status": "ok",
			"count":  5,
			"uint":   uint(10),
			"float":  2.5,
			"active": true,
			"nilval": nil,
			"list":   []any{"a", 1},
			"name":   "test",
		}
		res := formatValueWithFieldKeys(input, 1, wireToConst)
		if res == "" {
			t.Fatal("expected non-empty formatValueWithFieldKeys")
		}
	})

	t.Run("valueUsesFieldKeyLiterals and rulesUseFieldKeyLiterals", func(t *testing.T) {
		wireToConst := map[string]string{
			"status": "FieldKeyStatus",
		}
		ruleWithKey := map[string]any{"status": "ok"}
		ruleWithoutKey := map[string]any{"other": "ok"}

		if !valueUsesFieldKeyLiterals(ruleWithKey, wireToConst) {
			t.Error("expected true for rule with field key")
		}
		if !valueUsesFieldKeyLiterals([]any{ruleWithKey}, wireToConst) {
			t.Error("expected true for nested rule with field key")
		}
		if valueUsesFieldKeyLiterals(ruleWithoutKey, wireToConst) {
			t.Error("expected false for rule without field key")
		}

		if !rulesUseFieldKeyLiterals([]map[string]any{ruleWithKey}, wireToConst) {
			t.Error("expected true for rules list with field key")
		}
		if rulesUseFieldKeyLiterals([]map[string]any{ruleWithoutKey}, wireToConst) {
			t.Error("expected false for rules list without field key")
		}
	})

	t.Run("generateBuilderCode", func(t *testing.T) {
		rules := []map[string]any{
			{"source": "cli", "status": "active"},
		}
		code, err := generateBuilderCode(rules, "default_rules", "v1_0_0", "test_pkg", map[string]string{"status": "FieldKeyStatus"}, "github.com/zqk-os/zqk", "routing_builders")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code == "" {
			t.Fatal("expected non-empty code")
		}
	})

	t.Run("GenerateBuilderFromYAML", func(t *testing.T) {
		tempDir := t.TempDir()
		outDir := filepath.Join(tempDir, "pkg", "specbuilder", "routing_builders")
		if err := fileutil.MkdirAll(outDir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create outDir: %v", err)
		}
		_ = fileutil.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module github.com/zqk-os/zqk\n"), paths.FilePerm644)
		objectsDir := filepath.Join(tempDir, "pkg", "objects")
		if err := fileutil.MkdirAll(objectsDir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create objectsDir: %v", err)
		}
		_ = fileutil.WriteFile(filepath.Join(objectsDir, "field_keys.go"), []byte("package objects\n\nconst (\n\tFieldKeyStatus = \"status\"\n)\n"), paths.FilePerm644)

		yamlPath := filepath.Join(tempDir, "sample_routing.yaml")
		_ = fileutil.WriteFile(yamlPath, []byte("- source: cli\n  target: router\n"), paths.FilePerm644)

		err := GenerateBuilderFromYAML(yamlPath, outDir)
		if err != nil {
			t.Fatalf("unexpected GenerateBuilderFromYAML error: %v", err)
		}
	})
}
