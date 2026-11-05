package api_builders

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestToCamelCase(t *testing.T) {
	assert.Equal(t, "SchedulerMaintenanceApi", toCamelCase("scheduler_maintenance_api"))
	assert.Equal(t, "Simple", toCamelCase("simple"))
	assert.Equal(t, "ApiV2Test", toCamelCase("api_v2_test"))
}

func TestFormatValue(t *testing.T) {
	// String
	assert.Equal(t, `"hello"`, formatValue("hello", 0, "pkg", "file"))

	// Numbers
	assert.Equal(t, `42`, formatValue(42, 0, "pkg", "file"))
	assert.Equal(t, `100`, formatValue(uint(100), 0, "pkg", "file"))
	assert.Equal(t, `3.14`, formatValue(3.14, 0, "pkg", "file"))

	// Booleans
	assert.Equal(t, `true`, formatValue(true, 0, "pkg", "file"))
	assert.Equal(t, `false`, formatValue(false, 0, "pkg", "file"))

	// Nil
	assert.Equal(t, `nil`, formatValue(nil, 0, "pkg", "file"))

	// Slice
	sliceVal := []any{"a", 1}
	formattedSlice := formatValue(sliceVal, 0, "pkg", "file")
	assert.Contains(t, formattedSlice, "[]any{")
	assert.Contains(t, formattedSlice, `"a"`)
	assert.Contains(t, formattedSlice, `1`)

	// Map generic
	mapVal := map[string]any{
		"title": "my api",
	}
	formattedMap := formatValue(mapVal, 0, "pkg", "file")
	assert.Contains(t, formattedMap, `"title": "my api"`)

	// Map special: defaultVersionPackage + version "1.0.0"
	mapWithVersion := map[string]any{
		"version": defaultAPIYAMLVersion,
	}
	formattedVer := formatValue(mapWithVersion, 0, defaultVersionPackage, "file")
	assert.Contains(t, formattedVer, "APIYAMLVersionV1")

	// Map special: schedulerMaintenanceAPIFileName
	schedMap := map[string]any{
		objects.FieldKeyID:      "job-1",
		objects.FieldKeyJobType: "cleanup",
	}
	formattedSched := formatValue(schedMap, 0, "pkg", schedulerMaintenanceAPIFileName)
	assert.Contains(t, formattedSched, "objects.FieldKeyID:")
	assert.Contains(t, formattedSched, "objects.FieldKeyJobType:")

	// Fallback struct or channel
	type dummy struct{}
	assert.Contains(t, formatValue(dummy{}, 0, "pkg", "file"), "{}")
}

func TestSourceYAMLForCodegenHeader(t *testing.T) {
	rel := sourceYAMLForCodegenHeader("foo/bar.yaml")
	assert.NotEmpty(t, rel)
}

func TestGenerateBuilderFromYAML(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "test_api.yaml")
	yamlContent := []byte("title: Test API\nversion: 1.0.0\n")
	err := fileutil.WriteFile(yamlFile, yamlContent, paths.FilePerm600)
	require.NoError(t, err)

	outDir := filepath.Join(tmpDir, "api_builders")
	err = GenerateBuilderFromYAML(yamlFile, outDir)
	require.NoError(t, err)

	// Sibling directory bldr_api_v1 should have been created with test_api_builder.go
	genFile := filepath.Join(tmpDir, defaultVersionPackage, "test_api_builder.go")
	content, err := fileutil.ReadFile(genFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "package bldr_api_v1")
	assert.Contains(t, string(content), "TestApiBuilder")

	// Invalid YAML
	badYAML := filepath.Join(tmpDir, "bad.yaml")
	_ = fileutil.WriteFile(badYAML, []byte("::not-yaml"), paths.FilePerm600)
	err = GenerateBuilderFromYAML(badYAML, outDir)
	require.Error(t, err)

	// Missing YAML
	err = GenerateBuilderFromYAML(filepath.Join(tmpDir, "nonexistent.yaml"), outDir)
	require.Error(t, err)
}

func TestGenerateBuilderCode(t *testing.T) {
	api := map[string]any{
		"name": "sample",
	}
	code, err := generateBuilderCode(api, "sample_spec", "v1_0_0", "testpkg", "specs/sample.yaml")
	require.NoError(t, err)
	assert.Contains(t, code, "package testpkg")
	assert.Contains(t, code, "SampleSpecBuilder")
	assert.Contains(t, code, "NewSampleSpecBuilder()")
}
