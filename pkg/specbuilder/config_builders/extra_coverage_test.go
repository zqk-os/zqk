package config_builders

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type dummyConfigBuilder struct {
	fileName string
	version  string
	data     []byte
	err      error
}

func (d *dummyConfigBuilder) Build() ([]byte, error) {
	return d.data, d.err
}

func (d *dummyConfigBuilder) GetVersion() string {
	return d.version
}

func (d *dummyConfigBuilder) GetFileName() string {
	return d.fileName
}

func TestVersionedConfigBuilderRegistry(t *testing.T) {
	reg := NewVersionedConfigBuilderRegistry()

	b1 := &dummyConfigBuilder{fileName: "app_config", version: "v1_0_0", data: []byte("v1")}
	b2 := &dummyConfigBuilder{fileName: "app_config", version: "v1_2_0", data: []byte("v1.2")}
	b3 := &dummyConfigBuilder{fileName: "app_config", version: "v1_10_0", data: []byte("v1.10")}
	bCustom := &dummyConfigBuilder{fileName: "custom_config", version: "custom_ver", data: []byte("custom")}

	reg.Register(b1)
	reg.Register(b2)
	reg.Register(b3)
	reg.Register(bCustom)

	t.Run("GetBuilder existing", func(t *testing.T) {
		got, err := reg.GetBuilder("app_config", "v1_2_0")
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
		_, err := reg.GetBuilder("app_config", "v9_9_9")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("GetLatestVersion semver comparison", func(t *testing.T) {
		latest, err := reg.GetLatestVersion("app_config")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if latest != "v1_10_0" {
			t.Fatalf("expected v1_10_0, got %s", latest)
		}
	})

	t.Run("GetLatestVersion non-semver fallback", func(t *testing.T) {
		latest, err := reg.GetLatestVersion("custom_config")
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

		versions := reg.GetVersions("app_config")
		if len(versions) != 3 {
			t.Fatalf("expected 3 versions for app_config, got %d", len(versions))
		}

		if nilVersions := reg.GetVersions("nonexistent"); nilVersions != nil {
			t.Fatalf("expected nil for nonexistent file versions, got %v", nilVersions)
		}
	})
}

func TestBaseConfigBuilder(t *testing.T) {
	builder := NewBaseConfigBuilder("test_cfg", "v1_0_0")
	if builder.GetFileName() != "test_cfg" {
		t.Errorf("expected test_cfg, got %s", builder.GetFileName())
	}
	if builder.GetVersion() != "v1_0_0" {
		t.Errorf("expected v1_0_0, got %s", builder.GetVersion())
	}

	builder.SetConfig(map[string]any{"key": "value", "number": 42})
	data, err := builder.Build()
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty YAML output")
	}
}

func TestConfigGenerator(t *testing.T) {
	tempDir := t.TempDir()
	gen := NewConfigGenerator(tempDir)

	if err := gen.EnsureOutputDir(); err != nil {
		t.Fatalf("unexpected EnsureOutputDir error: %v", err)
	}

	b := &dummyConfigBuilder{fileName: "my_gen_config", version: "v1_0_0", data: []byte("foo: bar\n")}
	gen.registry.Register(b)

	t.Run("GenerateConfig", func(t *testing.T) {
		err := gen.GenerateConfig("my_gen_config", "v1_0_0")
		if err != nil {
			t.Fatalf("unexpected GenerateConfig error: %v", err)
		}
		content, err := fileutil.ReadFile(filepath.Join(tempDir, "my_gen_config.yaml"))
		if err != nil {
			t.Fatalf("expected file to exist: %v", err)
		}
		if string(content) != "foo: bar\n" {
			t.Fatalf("unexpected content: %s", string(content))
		}
	})

	t.Run("GenerateLatestConfig", func(t *testing.T) {
		err := gen.GenerateLatestConfig("my_gen_config")
		if err != nil {
			t.Fatalf("unexpected GenerateLatestConfig error: %v", err)
		}
	})

	t.Run("GenerateAllConfigs", func(t *testing.T) {
		err := gen.GenerateAllConfigs()
		if err != nil {
			t.Fatalf("unexpected GenerateAllConfigs error: %v", err)
		}
	})
}

func TestCodegenHelpers(t *testing.T) {
	t.Run("toCamelCase", func(t *testing.T) {
		cases := map[string]string{
			"paths_config":                 "PathsConfig",
			"scheduler_maintenance_config": "SchedulerMaintenanceConfig",
			"single":                       "Single",
			"":                             "",
		}
		for in, want := range cases {
			if got := toCamelCase(in); got != want {
				t.Errorf("toCamelCase(%q) = %q, want %q", in, got, want)
			}
		}
	})

	t.Run("formatValue types", func(t *testing.T) {
		input := map[string]any{
			"str":      "hello",
			"int_val":  123,
			"uint_val": uint(456),
			"flt_val":  3.14,
			"bool_val": true,
			"nil_val":  nil,
			"slice":    []any{"item1", 2},
			"version":  "1.0.0",
		}
		res := formatValue(input, 1, defaultVersionPackage, "test_config")
		if res == "" {
			t.Fatal("expected non-empty formatValue output")
		}

		// Test scheduler maintenance config key replacement
		schedInput := map[string]any{
			"id":       "JOB-1",
			"job_type": "cleanup",
		}
		resSched := formatValue(schedInput, 1, defaultVersionPackage, schedulerMaintenanceConfigFileName)
		if resSched == "" {
			t.Fatal("expected non-empty formatValue output")
		}
	})

	t.Run("generateBuilderCode", func(t *testing.T) {
		cfg := map[string]any{
			"name": "example",
		}
		code, err := generateBuilderCode(cfg, "example_config", "v1_0_0", "test_pkg", "spec/example.yaml")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code == "" {
			t.Fatal("expected non-empty code")
		}
	})

	t.Run("sourceYAMLForCodegenHeader", func(t *testing.T) {
		res := sourceYAMLForCodegenHeader("path/to/spec.yaml")
		if res == "" {
			t.Fatal("expected non-empty header path")
		}
	})

	t.Run("GenerateBuilderFromYAML", func(t *testing.T) {
		tempDir := t.TempDir()
		yamlPath := filepath.Join(tempDir, "sample_test_config.yaml")
		_ = fileutil.WriteFile(yamlPath, []byte("version: 1.0.0\nname: test\n"), paths.FilePerm644)

		outDir := filepath.Join(tempDir, "out")
		err := GenerateBuilderFromYAML(yamlPath, outDir)
		if err != nil {
			t.Fatalf("unexpected GenerateBuilderFromYAML error: %v", err)
		}
	})
}
