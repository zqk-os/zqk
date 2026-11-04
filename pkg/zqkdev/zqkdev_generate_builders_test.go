package zqkdev_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/zqkdev"
)

func createMockModuleWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/zqk-os/zqk\n\ngo 1.22\n"), 0o600))
	pkgObj := filepath.Join(dir, "pkg", "objects")
	require.NoError(t, os.MkdirAll(pkgObj, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgObj, "field_keys.go"), []byte("package objects\n\nconst (\n\tFieldKeyStatus = \"status\"\n\tFieldKeyEnabled = \"enabled\"\n)\n"), 0o600))
	return dir
}

// TestCycle244_ProjectRootAndCache exercises ProjectRootOrResolve and InvalidateDescriptorReadModelCache.
func TestCycle244_ProjectRootAndCache(t *testing.T) {

	// Explicit path provided
	res := zqkdev.ProjectRootOrResolve("/custom/project/root")
	assert.Equal(t, "/custom/project/root", res)

	// Empty path falls back to ResolveProjectRoot
	resolved := zqkdev.ProjectRootOrResolve("")
	assert.NotEmpty(t, resolved)

	// InvalidateDescriptorReadModelCache
	err := zqkdev.InvalidateDescriptorReadModelCache("/dummy/path")
	assert.NoError(t, err)
}

// TestCycle244_CollectGenerateItems exercises CollectGenerateItems with various directory entries.
func TestCycle244_CollectGenerateItems(t *testing.T) {

	tmpDir := t.TempDir()

	// Create test files
	validYaml := filepath.Join(tmpDir, "valid_item.yaml")
	require.NoError(t, os.WriteFile(validYaml, []byte("title: Valid Item\n"), 0o600))

	validYml := filepath.Join(tmpDir, "second_item.yml")
	require.NoError(t, os.WriteFile(validYml, []byte("title: Second Item\n"), 0o600))

	nonYaml := filepath.Join(tmpDir, "notes.txt")
	require.NoError(t, os.WriteFile(nonYaml, []byte("hello\n"), 0o600))

	appleDouble := filepath.Join(tmpDir, "._hidden.yaml")
	require.NoError(t, os.WriteFile(appleDouble, []byte("junk\n"), 0o600))

	gitDir := filepath.Join(tmpDir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD.yaml"), []byte("ref: refs/heads/main\n"), 0o600))

	placeholder := filepath.Join(tmpDir, "_placeholder.yaml")
	require.NoError(t, os.WriteFile(placeholder, []byte("placeholder\n"), 0o600))

	// Output destination
	outDir := t.TempDir()
	existingOutput := filepath.Join(outDir, "valid_item_out.go")
	require.NoError(t, os.WriteFile(existingOutput, []byte("// existing\n"), 0o600))

	logger := logging.GetLoggerFromProfile("human")

	// 1. Without overwrite - existing output should be skipped
	items, skipped, err := zqkdev.CollectGenerateItems(tmpDir, logger, false, func(baseName string, d os.DirEntry) (string, string) {
		return baseName, filepath.Join(outDir, baseName+"_out.go")
	})
	require.NoError(t, err)
	assert.Equal(t, 1, skipped)
	assert.Len(t, items, 1)
	assert.Equal(t, "second_item", items[0].BaseName)

	// 2. With overwrite - existing output should NOT be skipped
	itemsOverwrite, skippedOverwrite, err := zqkdev.CollectGenerateItems(tmpDir, logger, true, func(baseName string, d os.DirEntry) (string, string) {
		return baseName, filepath.Join(outDir, baseName+"_out.go")
	})
	require.NoError(t, err)
	assert.Equal(t, 0, skippedOverwrite)
	assert.Len(t, itemsOverwrite, 2)
}

// TestCycle244_HelperFunctions exercises cmdLogger, LogSummaryAndCheckErrors, and ShouldProcessYAMLDirEntry.
func TestCycle244_HelperFunctions(t *testing.T) {

	logger := logging.GetLoggerFromProfile("human")

	// LogSummaryAndCheckErrors
	assert.NoError(t, zqkdev.LogSummaryAndCheckErrors(logger, 5, 2, 0))
	assert.Error(t, zqkdev.LogSummaryAndCheckErrors(logger, 5, 2, 1))

	// ShouldProcessYAMLDirEntry edge cases
	skipDir, process := zqkdev.ShouldProcessYAMLDirEntry(nil, fmt.Errorf("walk err"))
	assert.False(t, skipDir)
	assert.False(t, process)

	skipDir, process = zqkdev.ShouldProcessYAMLDirEntry(nil, nil)
	assert.False(t, skipDir)
	assert.False(t, process)
}

// TestCycle244_BuilderCommandsConstruction verifies that all generator commands construct cleanly.
func TestCycle244_BuilderCommandsConstruction(t *testing.T) {

	cmds := []struct {
		name string
		cmd  *cobra.Command
	}{
		{"command-builders", zqkdev.NewGenerateCommandBuildersCmd()},
		{"config-builders", zqkdev.NewGenerateConfigBuildersCmd()},
		{"field-keys", zqkdev.NewGenerateFieldKeysCmd()},
		{"instance-builders", zqkdev.NewGenerateInstanceBuildersCmd()},
		{"lifecycle-builders", zqkdev.NewGenerateLifecycleBuildersCmd()},
		{"lifecycle-id-list", zqkdev.NewGenerateLifecycleIDListCmd()},
		{"profile-builders", zqkdev.NewGenerateProfileBuildersCmd()},
		{"routing-builders", zqkdev.NewGenerateRoutingBuildersCmd()},
		{"spec-index", zqkdev.NewGenerateSpecIndexCmd()},
		{"trait-builders", zqkdev.NewGenerateTraitBuildersCmd()},
	}

	for _, tc := range cmds {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assert.NotEmpty(t, tc.cmd.Use)
		})
	}
}

// TestCycle244_GenerateCommandBuilders tests NewGenerateCommandBuildersCmd thoroughly.
func TestCycle244_GenerateCommandBuilders(t *testing.T) {
	ctx := context.Background()

	t.Run("NonExistentSpecsDir", func(t *testing.T) {
		cmd := zqkdev.NewGenerateCommandBuildersCmd()
		cmd.SetArgs([]string{"--specs-dir", "/path/that/does/not/exist"})
		err := cmd.ExecuteContext(ctx)
		assert.Error(t, err)
	})

	t.Run("OutputDirCreationFailure", func(t *testing.T) {
		tmpDir := t.TempDir()
		specsDir := filepath.Join(tmpDir, "specs")
		require.NoError(t, os.MkdirAll(specsDir, 0o755))
		blockingFile := filepath.Join(tmpDir, "file_as_dir")
		require.NoError(t, os.WriteFile(blockingFile, []byte("x"), 0o600))

		cmd := zqkdev.NewGenerateCommandBuildersCmd()
		cmd.SetArgs([]string{"--specs-dir", specsDir, "--output-dir", filepath.Join(blockingFile, "sub")})
		err := cmd.ExecuteContext(ctx)
		assert.Error(t, err)
	})

	t.Run("SuccessAndFilterBranches", func(t *testing.T) {
		ws := createMockModuleWorkspace(t)
		specsDir := filepath.Join(ws, "specs")
		require.NoError(t, os.MkdirAll(specsDir, 0o755))
		schedulerDir := filepath.Join(specsDir, "scheduler")
		require.NoError(t, os.MkdirAll(schedulerDir, 0o755))
		outDir := filepath.Join(ws, "pkg", "cli")

		// Scheduler directory file should be skipped
		require.NoError(t, os.WriteFile(filepath.Join(schedulerDir, "sched.yaml"), []byte("name: sched\n"), 0o600))
		// AppleDouble file skipped
		require.NoError(t, os.WriteFile(filepath.Join(specsDir, "._cmd.yaml"), []byte("apple\n"), 0o600))
		// Non-yaml skipped
		require.NoError(t, os.WriteFile(filepath.Join(specsDir, "notes.txt"), []byte("notes\n"), 0o600))

		// Process CAS command spec (skipped unless --include-process-command-specs)
		casSpec := filepath.Join(specsDir, "CSPEC-test-v1.yaml")
		require.NoError(t, os.WriteFile(casSpec, []byte("id: CSPEC-test-v1\nkind: command_spec\n"), 0o600))

		// Valid CRUD command spec
		validCmd := filepath.Join(specsDir, "valid_cmd.yaml")
		validYAML := `name: sample
use: sample
short: Sample command
description: Long sample description
operation_type: read
`
		require.NoError(t, os.WriteFile(validCmd, []byte(validYAML), 0o600))

		// 1. Run without --include-process-command-specs and with --overwrite
		cmd := zqkdev.NewGenerateCommandBuildersCmd()
		cmd.SetArgs([]string{"--specs-dir", specsDir, "--output-dir", outDir, "--overwrite"})
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)

		// Verify output file generated
		expectedFile := filepath.Join(outDir, "bldr_cli_cmd_v1", "sample_command_builder.go")
		assert.FileExists(t, expectedFile)

		// 2. Run without --overwrite (should skip existing builder)
		cmd2 := zqkdev.NewGenerateCommandBuildersCmd()
		cmd2.SetArgs([]string{"--specs-dir", specsDir, "--output-dir", outDir})
		err = cmd2.ExecuteContext(ctx)
		assert.NoError(t, err)

		// 3. Run with --include-process-command-specs (valid CSPEC-test-v1 generates)
		cmd3 := zqkdev.NewGenerateCommandBuildersCmd()
		cmd3.SetArgs([]string{"--specs-dir", specsDir, "--output-dir", outDir, "--include-process-command-specs", "--overwrite"})
		err = cmd3.ExecuteContext(ctx)
		assert.NoError(t, err)

		// 4. Refuse numeric stem
		numericSpec := filepath.Join(specsDir, "1785199714348041000_411cd659.yaml")
		require.NoError(t, os.WriteFile(numericSpec, []byte("type: custom\n"), 0o600))
		cmdNumeric := zqkdev.NewGenerateCommandBuildersCmd()
		cmdNumeric.SetArgs([]string{"--specs-dir", specsDir, "--output-dir", outDir, "--overwrite"})
		err = cmdNumeric.ExecuteContext(ctx)
		assert.Error(t, err)
		_ = os.Remove(numericSpec)

		// 5. Invalid YAML error branch
		badYAML := filepath.Join(specsDir, "bad.yaml")
		require.NoError(t, os.WriteFile(badYAML, []byte("bad: [invalid\n"), 0o600))
		cmd4 := zqkdev.NewGenerateCommandBuildersCmd()
		cmd4.SetArgs([]string{"--specs-dir", specsDir, "--output-dir", outDir, "--overwrite"})
		err = cmd4.ExecuteContext(ctx)
		assert.Error(t, err)
	})
}

// TestCycle244_GenerateConfigBuilders tests NewGenerateConfigBuildersCmd thoroughly.
func TestCycle244_GenerateConfigBuilders(t *testing.T) {
	ctx := context.Background()

	t.Run("NonExistentConfigsDir", func(t *testing.T) {
		cmd := zqkdev.NewGenerateConfigBuildersCmd()
		cmd.SetArgs([]string{"--configs-dir", "/path/that/does/not/exist"})
		err := cmd.ExecuteContext(ctx)
		assert.Error(t, err)
	})

	t.Run("OutputDirCreationFailure", func(t *testing.T) {
		tmpDir := t.TempDir()
		configsDir := filepath.Join(tmpDir, "configs")
		require.NoError(t, os.MkdirAll(configsDir, 0o755))
		blockingFile := filepath.Join(tmpDir, "blocking_file")
		require.NoError(t, os.WriteFile(blockingFile, []byte("x"), 0o600))

		cmd := zqkdev.NewGenerateConfigBuildersCmd()
		cmd.SetArgs([]string{"--configs-dir", configsDir, "--output-dir", filepath.Join(blockingFile, "sub")})
		err := cmd.ExecuteContext(ctx)
		assert.Error(t, err)
	})

	t.Run("SuccessAndFilterBranches", func(t *testing.T) {
		tmpDir := t.TempDir()
		configsDir := filepath.Join(tmpDir, "configs")
		require.NoError(t, os.MkdirAll(configsDir, 0o755))
		outDir := filepath.Join(tmpDir, "pkg", "specbuilder", "config_builders")

		// Create files to exercise filters
		subDir := filepath.Join(configsDir, "sub")
		require.NoError(t, os.MkdirAll(subDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(configsDir, "._hidden_config.yaml"), []byte("apple"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(configsDir, "notes.txt"), []byte("text"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(configsDir, "not_a_config.yaml"), []byte("version: 1.0\n"), 0o600))
		// 64-char hashed filename ending in _config.yaml
		hashedFile := filepath.Join(configsDir, "1234567890123456789012345678901234567890123456789012345678901234_config.yaml")
		require.NoError(t, os.WriteFile(hashedFile, []byte("version: 1.0\n"), 0o600))

		// Valid config file
		validConfig := filepath.Join(configsDir, "sample_test_config.yaml")
		require.NoError(t, os.WriteFile(validConfig, []byte("version: 1.0.0\nname: sample_test\nenabled: true\n"), 0o600))

		// 1. First run with overwrite
		cmd := zqkdev.NewGenerateConfigBuildersCmd()
		cmd.SetArgs([]string{"--configs-dir", configsDir, "--output-dir", outDir, "--overwrite"})
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)

		// 2. Second run without overwrite (skips existing)
		cmd2 := zqkdev.NewGenerateConfigBuildersCmd()
		cmd2.SetArgs([]string{"--configs-dir", configsDir, "--output-dir", outDir})
		err = cmd2.ExecuteContext(ctx)
		assert.NoError(t, err)
	})
}

// TestCycle244_GenerateProfileBuilders tests NewGenerateProfileBuildersCmd thoroughly.
func TestCycle244_GenerateProfileBuilders(t *testing.T) {
	ctx := context.Background()

	t.Run("NonExistentProfilesDirIsNoOp", func(t *testing.T) {
		cmd := zqkdev.NewGenerateProfileBuildersCmd()
		cmd.SetArgs([]string{"--profiles-dir", "/path/that/does/not/exist"})
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)
	})

	t.Run("OutputDirCreationFailure", func(t *testing.T) {
		tmpDir := t.TempDir()
		profilesDir := filepath.Join(tmpDir, "profiles")
		require.NoError(t, os.MkdirAll(profilesDir, 0o755))
		blockingFile := filepath.Join(tmpDir, "blocking_file")
		require.NoError(t, os.WriteFile(blockingFile, []byte("x"), 0o600))

		cmd := zqkdev.NewGenerateProfileBuildersCmd()
		cmd.SetArgs([]string{"--profiles-dir", profilesDir, "--output-dir", filepath.Join(blockingFile, "sub")})
		err := cmd.ExecuteContext(ctx)
		assert.Error(t, err)
	})

	t.Run("SuccessAndFilterBranches", func(t *testing.T) {
		ws := createMockModuleWorkspace(t)
		profilesDir := filepath.Join(ws, "profiles")
		require.NoError(t, os.MkdirAll(profilesDir, 0o755))
		outDir := filepath.Join(ws, "pkg", "specbuilder", "profile_builders")

		// Filter items
		subDir := filepath.Join(profilesDir, "sub")
		require.NoError(t, os.MkdirAll(subDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "._hidden.yaml"), []byte("apple"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "notes.txt"), []byte("text"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "_placeholder.yaml"), []byte("p"), 0o600))
		hashedFile := filepath.Join(profilesDir, "1234567890123456789012345678901234567890123456789012345678901234.yaml")
		require.NoError(t, os.WriteFile(hashedFile, []byte("h"), 0o600))

		// Valid profile
		validProfile := filepath.Join(profilesDir, "test_prof.yaml")
		validYAML := `schema_version: "1.0.0"
kind: profile
type: cli_context
metadata:
  name: test_prof
spec:
  enabled: true
`
		require.NoError(t, os.WriteFile(validProfile, []byte(validYAML), 0o600))

		// 1. Run with overwrite
		cmd := zqkdev.NewGenerateProfileBuildersCmd()
		cmd.SetArgs([]string{"--profiles-dir", profilesDir, "--output-dir", outDir, "--overwrite"})
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)

		// 2. Run without overwrite (skips existing)
		cmd2 := zqkdev.NewGenerateProfileBuildersCmd()
		cmd2.SetArgs([]string{"--profiles-dir", profilesDir, "--output-dir", outDir})
		err = cmd2.ExecuteContext(ctx)
		assert.NoError(t, err)
	})
}

// TestCycle244_GenerateRoutingBuilders tests NewGenerateRoutingBuildersCmd thoroughly.
func TestCycle244_GenerateRoutingBuilders(t *testing.T) {
	ctx := context.Background()

	t.Run("NonExistentRulesDir", func(t *testing.T) {
		cmd := zqkdev.NewGenerateRoutingBuildersCmd()
		cmd.SetArgs([]string{"--rules-dir", "/path/that/does/not/exist"})
		err := cmd.ExecuteContext(ctx)
		assert.Error(t, err)
	})

	t.Run("OutputDirCreationFailure", func(t *testing.T) {
		tmpDir := t.TempDir()
		rulesDir := filepath.Join(tmpDir, "rules")
		require.NoError(t, os.MkdirAll(rulesDir, 0o755))
		blockingFile := filepath.Join(tmpDir, "blocking_file")
		require.NoError(t, os.WriteFile(blockingFile, []byte("x"), 0o600))

		cmd := zqkdev.NewGenerateRoutingBuildersCmd()
		cmd.SetArgs([]string{"--rules-dir", rulesDir, "--output-dir", filepath.Join(blockingFile, "sub")})
		err := cmd.ExecuteContext(ctx)
		assert.Error(t, err)
	})

	t.Run("SuccessAndFilterBranches", func(t *testing.T) {
		ws := createMockModuleWorkspace(t)
		rulesDir := filepath.Join(ws, "rules")
		require.NoError(t, os.MkdirAll(rulesDir, 0o755))
		outDir := filepath.Join(ws, "pkg", "specbuilder", "routing_builders")

		// Filter items
		subDir := filepath.Join(rulesDir, "sub")
		require.NoError(t, os.MkdirAll(subDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(rulesDir, "._hidden.yaml"), []byte("apple"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(rulesDir, "notes.txt"), []byte("text"), 0o600))
		hashedFile := filepath.Join(rulesDir, "1234567890123456789012345678901234567890123456789012345678901234.yaml")
		require.NoError(t, os.WriteFile(hashedFile, []byte("h"), 0o600))

		// Valid routing rule
		validRule := filepath.Join(rulesDir, "sample_rules.yaml")
		validYAML := `- route: test
  destination: sink
`
		require.NoError(t, os.WriteFile(validRule, []byte(validYAML), 0o600))

		// 1. Run with overwrite
		cmd := zqkdev.NewGenerateRoutingBuildersCmd()
		cmd.SetArgs([]string{"--rules-dir", rulesDir, "--output-dir", outDir, "--overwrite"})
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)

		// 2. Run without overwrite (skips existing)
		cmd2 := zqkdev.NewGenerateRoutingBuildersCmd()
		cmd2.SetArgs([]string{"--rules-dir", rulesDir, "--output-dir", outDir})
		err = cmd2.ExecuteContext(ctx)
		assert.NoError(t, err)
	})
}

// TestCycle244_GenerateTraitBuilders tests NewGenerateTraitBuildersCmd thoroughly.
func TestCycle244_GenerateTraitBuilders(t *testing.T) {
	ctx := context.Background()

	t.Run("NonExistentTraitsDir", func(t *testing.T) {
		cmd := zqkdev.NewGenerateTraitBuildersCmd()
		cmd.SetArgs([]string{"--traits-dir", "/path/that/does/not/exist"})
		err := cmd.ExecuteContext(ctx)
		assert.Error(t, err)
	})

	t.Run("OutputDirCreationFailure", func(t *testing.T) {
		tmpDir := t.TempDir()
		traitsDir := filepath.Join(tmpDir, "traits")
		require.NoError(t, os.MkdirAll(traitsDir, 0o755))
		blockingFile := filepath.Join(tmpDir, "blocking_file")
		require.NoError(t, os.WriteFile(blockingFile, []byte("x"), 0o600))

		cmd := zqkdev.NewGenerateTraitBuildersCmd()
		cmd.SetArgs([]string{"--traits-dir", traitsDir, "--output-dir", filepath.Join(blockingFile, "sub")})
		err := cmd.ExecuteContext(ctx)
		assert.Error(t, err)
	})

	t.Run("SuccessAndFilterAndStatusBranches", func(t *testing.T) {
		ws := createMockModuleWorkspace(t)
		traitsDir := filepath.Join(ws, "traits")
		require.NoError(t, os.MkdirAll(traitsDir, 0o755))
		outDir := filepath.Join(ws, "pkg", "specbuilder", "trait_builders")

		// Filter items
		subDir := filepath.Join(traitsDir, "sub")
		require.NoError(t, os.MkdirAll(subDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(traitsDir, "._hidden.yaml"), []byte("apple"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(traitsDir, "notes.txt"), []byte("text"), 0o600))
		hashedFile := filepath.Join(traitsDir, "1234567890123456789012345678901234567890123456789012345678901234.yaml")
		require.NoError(t, os.WriteFile(hashedFile, []byte("h"), 0o600))

		// Valid trait
		validTrait := filepath.Join(traitsDir, "sample_trait.yaml")
		validYAML := `name: sample_trait
description: Sample trait
status: active
`
		require.NoError(t, os.WriteFile(validTrait, []byte(validYAML), 0o600))

		// Inactive trait (triggers strings.Contains(err.Error(), "status") skip branch)
		inactiveTrait := filepath.Join(traitsDir, "inactive_trait.yaml")
		inactiveYAML := `name: inactive_trait
status: inactive
`
		require.NoError(t, os.WriteFile(inactiveTrait, []byte(inactiveYAML), 0o600))

		// 1. Run with overwrite
		cmd := zqkdev.NewGenerateTraitBuildersCmd()
		cmd.SetArgs([]string{"--traits-dir", traitsDir, "--output-dir", outDir, "--overwrite"})
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)

		// 2. Run without overwrite (skips existing)
		cmd2 := zqkdev.NewGenerateTraitBuildersCmd()
		cmd2.SetArgs([]string{"--traits-dir", traitsDir, "--output-dir", outDir})
		err = cmd2.ExecuteContext(ctx)
		assert.NoError(t, err)

		// 3. Error branch (invalid YAML in trait)
		badTrait := filepath.Join(traitsDir, "bad.yaml")
		require.NoError(t, os.WriteFile(badTrait, []byte("bad: [unclosed\n"), 0o600))
		cmd3 := zqkdev.NewGenerateTraitBuildersCmd()
		cmd3.SetArgs([]string{"--traits-dir", traitsDir, "--output-dir", outDir, "--overwrite"})
		err = cmd3.ExecuteContext(ctx)
		assert.Error(t, err)
	})
}

// TestCycle244_GenerateInstanceBuilders tests NewGenerateInstanceBuildersCmd thoroughly.
func TestCycle244_GenerateInstanceBuilders(t *testing.T) {
	ctx := context.Background()

	t.Run("NonExistentSpecsDir", func(t *testing.T) {
		cmd := zqkdev.NewGenerateInstanceBuildersCmd()
		cmd.SetArgs([]string{"--specs-dir", "/path/that/does/not/exist"})
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)
	})

	t.Run("OutputDirCreationFailure", func(t *testing.T) {
		tmpDir := t.TempDir()
		specsDir := filepath.Join(tmpDir, "specs")
		require.NoError(t, os.MkdirAll(specsDir, 0o755))
		blockingFile := filepath.Join(tmpDir, "blocking_file")
		require.NoError(t, os.WriteFile(blockingFile, []byte("x"), 0o600))

		cmd := zqkdev.NewGenerateInstanceBuildersCmd()
		cmd.SetArgs([]string{"--specs-dir", specsDir, "--output-dir", filepath.Join(blockingFile, "sub")})
		err := cmd.ExecuteContext(ctx)
		assert.Error(t, err)
	})

	t.Run("SuccessAndFilterBranches", func(t *testing.T) {
		tmpDir := t.TempDir()
		specsDir := filepath.Join(tmpDir, "specs")
		require.NoError(t, os.MkdirAll(specsDir, 0o755))
		outDir := filepath.Join(tmpDir, "pkg", "specbuilder", "instance_builders")

		// Create a valid spec
		validSpec := filepath.Join(specsDir, "widget.yaml")
		validYAML := `schema_version: "1.0.0"
ontology: widget
fields:
  title:
    type: string
`
		require.NoError(t, os.WriteFile(validSpec, []byte(validYAML), 0o600))

		// 1. Run with overwrite
		cmd := zqkdev.NewGenerateInstanceBuildersCmd()
		cmd.SetArgs([]string{"--specs-dir", specsDir, "--output-dir", outDir, "--overwrite"})
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)

		// 2. Run without overwrite (skips existing)
		cmd2 := zqkdev.NewGenerateInstanceBuildersCmd()
		cmd2.SetArgs([]string{"--specs-dir", specsDir, "--output-dir", outDir})
		err = cmd2.ExecuteContext(ctx)
		assert.NoError(t, err)
	})
}

// TestCycle244_GenerateLifecycleBuilders tests NewGenerateLifecycleBuildersCmd thoroughly.
func TestCycle244_GenerateLifecycleBuilders(t *testing.T) {
	ctx := context.Background()

	t.Run("NonExistentLifecyclesDir", func(t *testing.T) {
		cmd := zqkdev.NewGenerateLifecycleBuildersCmd()
		cmd.SetArgs([]string{"--lifecycles-dir", "/path/that/does/not/exist"})
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)
	})

	t.Run("OutputDirCreationFailure", func(t *testing.T) {
		tmpDir := t.TempDir()
		lifecyclesDir := filepath.Join(tmpDir, "lifecycles")
		require.NoError(t, os.MkdirAll(lifecyclesDir, 0o755))
		blockingFile := filepath.Join(tmpDir, "blocking_file")
		require.NoError(t, os.WriteFile(blockingFile, []byte("x"), 0o600))

		cmd := zqkdev.NewGenerateLifecycleBuildersCmd()
		cmd.SetArgs([]string{"--lifecycles-dir", lifecyclesDir, "--output-dir", filepath.Join(blockingFile, "sub")})
		err := cmd.ExecuteContext(ctx)
		assert.Error(t, err)
	})

	t.Run("SuccessAndFilterBranches", func(t *testing.T) {
		ws := createMockModuleWorkspace(t)
		lifecyclesDir := filepath.Join(ws, "lifecycles")
		require.NoError(t, os.MkdirAll(lifecyclesDir, 0o755))
		outDir := filepath.Join(ws, "pkg", "specbuilder", "lifecycle_builders")

		// Create a valid lifecycle YAML
		validLC := filepath.Join(lifecyclesDir, "sample_lifecycle.yaml")
		validYAML := `object_type: sample
statuses:
  - value: active
`
		require.NoError(t, os.WriteFile(validLC, []byte(validYAML), 0o600))

		// 1. Run with overwrite
		cmd := zqkdev.NewGenerateLifecycleBuildersCmd()
		cmd.SetArgs([]string{"--lifecycles-dir", lifecyclesDir, "--output-dir", outDir, "--overwrite"})
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)

		// 2. Run without overwrite (skips existing)
		cmd2 := zqkdev.NewGenerateLifecycleBuildersCmd()
		cmd2.SetArgs([]string{"--lifecycles-dir", lifecyclesDir, "--output-dir", outDir})
		err = cmd2.ExecuteContext(ctx)
		assert.NoError(t, err)
	})
}

// TestCycle244_GenerateFieldKeys tests NewGenerateFieldKeysCmd thoroughly.
func TestCycle244_GenerateFieldKeys(t *testing.T) {
	ctx := context.Background()

	t.Run("InvalidSpecsDir", func(t *testing.T) {
		cmd := zqkdev.NewGenerateFieldKeysCmd()
		cmd.SetArgs([]string{"--specs-dir", "/path/that/does/not/exist", "--output", filepath.Join(t.TempDir(), "field_keys.go")})
		err := cmd.ExecuteContext(ctx)
		assert.Error(t, err)
	})

	t.Run("UnwritableOutput", func(t *testing.T) {
		emptyDir := t.TempDir()
		cmd := zqkdev.NewGenerateFieldKeysCmd()
		cmd.SetArgs([]string{"--specs-dir", emptyDir, "--output", "/dev/null/impossible/field_keys.go"})
		err := cmd.ExecuteContext(ctx)
		assert.Error(t, err)
	})

	t.Run("RelativeOutputPath", func(t *testing.T) {
		emptyDir := t.TempDir()
		subDir := t.TempDir()
		t.Chdir(subDir)

		cmd := zqkdev.NewGenerateFieldKeysCmd()
		cmd.SetArgs([]string{"--specs-dir", emptyDir, "--output", "rel_field_keys.go"})
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)
		assert.FileExists(t, filepath.Join(subDir, "rel_field_keys.go"))
	})

	t.Run("DefaultSpecsDirAndContextProjectRoot", func(t *testing.T) {
		outFile := filepath.Join(t.TempDir(), "field_keys.go")

		cmd := zqkdev.NewGenerateFieldKeysCmd()
		cmd.SetArgs([]string{"--output", outFile})
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)
		assert.FileExists(t, outFile)
	})
}

// TestCycle244_GenerateSpecIndex tests NewGenerateSpecIndexCmd thoroughly.
func TestCycle244_GenerateSpecIndex(t *testing.T) {
	ctx := context.Background()

	t.Run("InvalidSpecsDir", func(t *testing.T) {
		cmd := zqkdev.NewGenerateSpecIndexCmd()
		cmd.SetArgs([]string{"--specs-dir", "/path/that/does/not/exist", "--output", filepath.Join(t.TempDir(), "spec_index.json")})
		err := cmd.ExecuteContext(ctx)
		assert.Error(t, err)
	})

	t.Run("WithRealSpecsAndCustomOutput", func(t *testing.T) {
		root := zqkdev.ProjectRootOrResolve("")
		specsDir := filepath.Join(root, ".zqk", "specs", "objects")
		if _, err := os.Stat(specsDir); os.IsNotExist(err) {
			t.Skip("real specs dir not present")
		}

		outFile := filepath.Join(t.TempDir(), "spec_index.json")
		cmd := zqkdev.NewGenerateSpecIndexCmd()
		cmd.SetArgs([]string{"--specs-dir", specsDir, "--output", outFile})
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)
		assert.FileExists(t, outFile)
	})

	t.Run("WithDefaultSpecsDir", func(t *testing.T) {
		outFile := filepath.Join(t.TempDir(), "spec_index.json")
		cmd := zqkdev.NewGenerateSpecIndexCmd()
		cmd.SetArgs([]string{"--output", outFile})
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)
		assert.FileExists(t, outFile)
	})
}

// TestCycle244_GenerateLifecycleIDList_AllCases tests all branches of runGenerateLifecycleIDList.
func TestCycle244_GenerateLifecycleIDList_AllCases(t *testing.T) {
	ctx := context.Background()

	t.Run("StorageFactoryError", func(t *testing.T) {
		tmpDir := t.TempDir()
		// No .zqk or storage layout inside tmpDir
		t.Chdir(tmpDir)

		cmd := zqkdev.NewGenerateLifecycleIDListCmd()
		cmd.SetArgs([]string{"--output", filepath.Join(tmpDir, "out.yaml")})
		err := cmd.ExecuteContext(ctx)
		// Missing storage layout returns error
		assert.Error(t, err)
	})

	t.Run("LifecycleIDList_EmptyNoOldIDs", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("ZQK_TEST_ROOT", tmpDir)
		t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".zqk", "process"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".zqk", "wal"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".zqk", "specs", "_internal", "migrations"), 0o755))

		t.Chdir(tmpDir)

		outFile := filepath.Join(tmpDir, "lifecycle_ids.yaml")
		cmd := zqkdev.NewGenerateLifecycleIDListCmd()
		cmd.SetArgs([]string{"--output", outFile})
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)
	})

	t.Run("LifecycleIDList_WithOldIDsAndCustomOutput", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("ZQK_TEST_ROOT", tmpDir)
		t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
		lifecycleDir := filepath.Join(tmpDir, ".zqk", "process", "lifecycles")
		require.NoError(t, os.MkdirAll(lifecycleDir, 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".zqk", "wal"), 0o755))

		oldIDFile := filepath.Join(lifecycleDir, "LIFECYCLE-task-v1.yaml")
		require.NoError(t, os.WriteFile(oldIDFile, []byte("id: LIFECYCLE-task-v1\nkind: lifecycle\ntitle: Task Lifecycle\n"), 0o600))

		t.Chdir(tmpDir)

		outFile := filepath.Join(tmpDir, "migrated_ids.yaml")
		cmd := zqkdev.NewGenerateLifecycleIDListCmd()
		cmd.SetArgs([]string{"--output", outFile})
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)

		data, err := os.ReadFile(outFile)
		assert.NoError(t, err)
		assert.Contains(t, string(data), "LIFECYCLE-task-v1")
	})

	t.Run("LifecycleIDList_WithOldIDsAndDefaultOutput", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("ZQK_TEST_ROOT", tmpDir)
		t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
		lifecycleDir := filepath.Join(tmpDir, ".zqk", "process", "lifecycles")
		require.NoError(t, os.MkdirAll(lifecycleDir, 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".zqk", "wal"), 0o755))

		oldIDFile := filepath.Join(lifecycleDir, "LIFECYCLE-task-v1.yaml")
		require.NoError(t, os.WriteFile(oldIDFile, []byte("id: LIFECYCLE-task-v1\nkind: lifecycle\ntitle: Task Lifecycle\n"), 0o600))

		t.Chdir(tmpDir)

		cmd := zqkdev.NewGenerateLifecycleIDListCmd()
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)

		defaultOut := filepath.Join(tmpDir, paths.ProcessInternalDir, "migrations", "lifecycle_ids_to_migrate.yaml")
		assert.FileExists(t, defaultOut)
	})
}
