package utility

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/cmd/zqk/system"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestCycle141_Migrate_ExecutionAndErrorBranches(t *testing.T) {
	// 1. Invalid version constraint
	cmd := NewMigrateCmd()
	cmd.SetArgs([]string{"--version-constraint", "invalid!!version"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid version constraint")

	// 2. Binary not available when PATH doesn't have zqk-migrate
	cleanPath := t.TempDir()
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", cleanPath)

	cmd2 := NewMigrateCmd()
	cmd2.SetArgs([]string{"--source", "/tmp/nonexistent"})
	err = cmd2.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Migration tool not available")

	// 3. Binary found in PATH but integrity check fails
	dummyBin := filepath.Join(cleanPath, "zqk-migrate")
	require.NoError(t, fileutil.WriteFile(dummyBin, []byte("#!/bin/sh\necho 1.0.0\n"), paths.FilePerm755))

	cmd3 := NewMigrateCmd()
	cmd3.SetArgs([]string{"--source", "/tmp/src", "--target", "memgraph", "--phase", "all"})
	err = cmd3.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "migration binary integrity check failed")

	t.Setenv("PATH", origPath)
}

func TestCycle141_Version_ExecutableAndMetadata(t *testing.T) {
	// 1. getExecutableName
	execName := getExecutableName()
	assert.NotEmpty(t, execName)

	// 2. RunVersion with versionInfo set
	SetVersionInfo("2.1.0", "2026-10-23T00:00:00Z", "abcdef123456")
	var buf bytes.Buffer
	cmd := &cobra.Command{Use: "version"}
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(ctx)

	RunVersion(cmd, nil)
	out := buf.String()
	assert.Contains(t, out, "2.1.0")
	assert.Contains(t, out, "abcdef123456")
	assert.Contains(t, out, "2026-10-23T00:00:00Z")

	// 3. RunVersion with versionInfo nil
	versionInfo = nil
	buf.Reset()
	rootCmd := &cobra.Command{Use: "zqk", Version: "9.9.9"}
	rootCmd.AddCommand(cmd)
	RunVersion(cmd, nil)
	assert.Contains(t, buf.String(), "9.9.9")

	// 4. Builders
	rootBuilder := NewRootVersionCmd()
	assert.NotNil(t, rootBuilder)
	assert.Equal(t, "version", rootBuilder.Use)

	utilBuilder := NewVersionCmd()
	assert.NotNil(t, utilBuilder)
	assert.Equal(t, "version", utilBuilder.Use)
}

func TestCycle141_ScenarioBuilder_ValidateObjectBeforeCreation(t *testing.T) {
	sb := &ScenarioBuilder{config: &ScenarioBuilderConfig{}}

	// Case 1: Missing kind and empty kind parameter returns error
	emptyObj := make(map[string]any)
	err := sb.validateObjectBeforeCreation(emptyObj, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "object missing required field: kind")

	// Case 2: Missing kind with parameter sets kind and default schema
	objWithParam := make(map[string]any)
	require.NoError(t, sb.validateObjectBeforeCreation(objWithParam, "goal"))
	assert.Equal(t, "goal", objWithParam[objects.FieldKeyKind])
	assert.Equal(t, objects.DefaultSchemaVersion, objWithParam[objects.FieldKeySchemaVersion])

	// Case 3: Empty string ID is stripped
	objEmptyID := map[string]any{
		objects.FieldKeyKind: "goal",
		objects.FieldKeyID:   "",
	}
	require.NoError(t, sb.validateObjectBeforeCreation(objEmptyID, "goal"))
	_, hasID := objEmptyID[objects.FieldKeyID]
	assert.False(t, hasID)

	// Case 4: Account with legacy prefix has ID stripped
	accountObj := map[string]any{
		objects.FieldKeyKind: objects.KindAccount,
		objects.FieldKeyID:   "account:admin",
	}
	require.NoError(t, sb.validateObjectBeforeCreation(accountObj, objects.KindAccount))
	_, hasAccountID := accountObj[objects.FieldKeyID]
	assert.False(t, hasAccountID)
}

func TestCycle141_ScenarioBuilder_SetFieldsDirectly(t *testing.T) {
	sb := &ScenarioBuilder{config: &ScenarioBuilderConfig{}}

	obj := make(map[string]any)
	require.NoError(t, sb.setFieldsDirectly(obj, "requirement"))

	assert.Equal(t, objects.DefaultSchemaVersion, obj[objects.FieldKeySchemaVersion])
	assert.Equal(t, "zqk", obj[objects.FieldKeyOriginProject])
	assert.Equal(t, "zqk", obj[objects.FieldKeyOriginSystem])
	assert.Equal(t, "zqk:kernel", obj[objects.FieldKeyNamespaceID])
	assert.Equal(t, pkgctx.SystemAccountID, obj[objects.FieldKeyCreatedBy])
	assert.Equal(t, pkgctx.SystemAccountID, obj[objects.FieldKeyUpdatedBy])
	assert.NotEmpty(t, obj[objects.FieldKeyCreatedAt])
	assert.NotEmpty(t, obj[objects.FieldKeyUpdatedAt])

	// Account kind strips legacy prefix
	accObj := map[string]any{objects.FieldKeyID: "account:user1"}
	require.NoError(t, sb.setFieldsDirectly(accObj, objects.KindAccount))
	assert.Nil(t, accObj[objects.FieldKeyID])

	// Empty ID is removed
	emptyIDObj := map[string]any{objects.FieldKeyID: ""}
	require.NoError(t, sb.setFieldsDirectly(emptyIDObj, "goal"))
	assert.Nil(t, emptyIDObj[objects.FieldKeyID])
}

func TestCycle141_ScenarioBuilder_PrepareObjectFromDataFile(t *testing.T) {
	sb := &ScenarioBuilder{config: &ScenarioBuilderConfig{}}

	// Object with ID and valid fields
	objWithID := map[string]any{
		objects.FieldKeyID:            "GOAL-DEMO-001",
		objects.FieldKeyKind:          "goal",
		objects.FieldKeyTitle:         "Demonstration Goal",
		objects.FieldKeyStatus:        "proposed",
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	err := sb.prepareObjectFromDataFile(objWithID, "goal")
	require.NoError(t, err)
	assert.Equal(t, "GOAL-DEMO-001", objWithID[objects.FieldKeyID])

	// Object without ID triggers auto-generation prep
	objWithoutID := map[string]any{
		objects.FieldKeyTitle: "No ID Goal",
	}
	err = sb.prepareObjectFromDataFile(objWithoutID, "goal")
	require.NoError(t, err)
	assert.Equal(t, "goal", objWithoutID[objects.FieldKeyKind])
	assert.Equal(t, objects.DefaultSchemaVersion, objWithoutID[objects.FieldKeySchemaVersion])
}

func newTestScenarioBuilder(t *testing.T, targetDir string) *ScenarioBuilder {
	t.Helper()
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	return &ScenarioBuilder{
		projectRoot: targetDir,
		config:      &ScenarioBuilderConfig{TargetDir: targetDir},
		coordinator: coordinator,
		logger:      logging.NewEventLogger(pkgctx.NewSystemContext()),
	}
}

func TestCycle141_ScenarioBuilder_DataLoader_LoadErrors(t *testing.T) {
	builder := newTestScenarioBuilder(t, t.TempDir())

	// 1. Missing file error
	flagsMissing := &ScenarioBuilderFlags{
		DataFile: filepath.Join(t.TempDir(), "nonexistent.yaml"),
	}
	err := handleLoadDataFile(nil, builder, flagsMissing)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read data file")

	// 2. Corrupt YAML syntax
	corruptFile := filepath.Join(t.TempDir(), "corrupt.yaml")
	require.NoError(t, fileutil.WriteFile(corruptFile, []byte("key: [unterminated\n"), paths.FilePerm644))
	flagsCorrupt := &ScenarioBuilderFlags{DataFile: corruptFile}
	err = handleLoadDataFile(nil, builder, flagsCorrupt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse YAML data file")

	// 3. Valid YAML list
	validFile := filepath.Join(t.TempDir(), "valid.yaml")
	validYAML := "- id: GOAL-DEMO-001\n  kind: goal\n  title: Test Goal\n"
	require.NoError(t, fileutil.WriteFile(validFile, []byte(validYAML), paths.FilePerm644))
	flagsValid := &ScenarioBuilderFlags{DataFile: validFile, Force: true}
	err = handleLoadDataFile(nil, builder, flagsValid)
	require.NoError(t, err)
	assert.Len(t, builder.dataFileObjects, 1)
	assert.True(t, builder.force)
}

func TestCycle141_ScenarioBuilder_InfrastructureConfig(t *testing.T) {
	sb := &ScenarioBuilder{
		config: &ScenarioBuilderConfig{TargetDir: t.TempDir()},
	}

	// 1. copyScenarioBuilderProfile creates profile YAML
	targetDir := t.TempDir()
	require.NoError(t, sb.copyScenarioBuilderProfile(targetDir))
	profileFile := filepath.Join(targetDir, "pkg", "cli", "profiles", ScenarioBuilderProfileName+".yaml")
	data, err := fileutil.ReadFile(profileFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), ScenarioBuilderProfileName)

	// 2. copyConfigFile error on nonexistent source
	err = sb.copyConfigFile(filepath.Join(t.TempDir(), "missing.yaml"), filepath.Join(targetDir, "target.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open source file")

	// 3. copyConfigFile success and timestamp preservation
	srcFile := filepath.Join(t.TempDir(), "source.yaml")
	dstFile := filepath.Join(targetDir, "dest", "nested", "copied.yaml")
	require.NoError(t, fileutil.WriteFile(srcFile, []byte("content: true\n"), paths.FilePerm644))
	require.NoError(t, sb.copyConfigFile(srcFile, dstFile))
	dstData, err := fileutil.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, "content: true\n", string(dstData))

	// 4. copyBootstrapConfigFiles with no configs found
	err = sb.copyBootstrapConfigFiles(targetDir)
	assert.Error(t, err)
}

func TestCycle141_ScenarioBuilder_LifecycleCreator_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	sb := newTestScenarioBuilder(t, tmpDir)
	ctx := pkgctx.NewSystemContext()

	// 1. Directory does not exist -> returns nil (skips)
	require.NoError(t, sb.createLifecycleObjects(ctx))

	// 2. Directory exists with non-yaml and .bak files -> skipped
	lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
	require.NoError(t, fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(lifecyclesDir, "ignore.txt"), []byte("text"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(lifecyclesDir, "test_lifecycle.yaml.bak"), []byte("backup"), paths.FilePerm644))
	require.NoError(t, sb.createLifecycleObjects(ctx))

	// 3. lifecycleToObjectMap with status properties
	sampleLifecycle := &objects.Lifecycle{
		ObjectType: "custom_kind",
		Extends:    "base_object",
		StatusMapping: map[string]string{
			"done": "completed",
		},
		Statuses: []objects.Status{
			{
				Value:         "active",
				Display:       "Active",
				Origin:        true,
				Terminal:      false,
				Archive:       false,
				System:        true,
				Preconditions: []string{"valid_inputs"},
				Description:   "Active execution state",
			},
			{
				Value:    "done",
				Display:  "Done",
				Terminal: true,
			},
		},
	}
	objMap, err := lifecycleToObjectMap(sampleLifecycle, "custom_kind", "001")
	require.NoError(t, err)
	assert.Equal(t, "LIFECYCLE-CUSTOM-KIND-001", objMap[objects.FieldKeyID])
	assert.Equal(t, "custom_kind", objMap[objects.FieldKeyObjectType])
}

func TestCycle141_FixRegistration_EdgeCases(t *testing.T) {
	// 1. notifyScanning
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(ctx)

	notifyScanning(cmd, true)
	assert.Empty(t, buf.String())

	notifyScanning(cmd, false)
	assert.Contains(t, buf.String(), "Scanning objects")

	// 2. getPrefixFromID
	assert.Equal(t, "GOAL-", getPrefixFromID("GOAL-001"))
	assert.Equal(t, "CRIT-", getPrefixFromID("CRIT-ABC-123"))
	assert.Equal(t, "", getPrefixFromID("NOPREFIX"))

	// 3. checkRegistrationIssue when file does not exist
	projRoot := t.TempDir()
	validator := validation.GetIDValidator()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	issue := checkRegistrationIssue(projRoot, "NONEXISTENT-001", validator, logger)
	assert.Nil(t, issue)

	// 4. findKindRegistrationIssues with empty dir
	issues := findKindRegistrationIssues(projRoot, "unknown_kind", validator, logger)
	assert.Empty(t, issues)

	// 5. checkRegistrationIssue with object having invalid prefix
	goalDir := filepath.Join(projRoot, paths.ProcessGoalsDir)
	require.NoError(t, fileutil.MkdirAll(goalDir, paths.DirPerm755))
	invalidGoalYAML := "id: INVALID-001\nkind: goal\ntitle: Invalid ID Goal\n"
	require.NoError(t, fileutil.WriteFile(filepath.Join(goalDir, "INVALID-001.yaml"), []byte(invalidGoalYAML), paths.FilePerm644))

	issue = checkRegistrationIssue(projRoot, "INVALID-001", validator, logger)
	if issue != nil {
		assert.Equal(t, "INVALID-001", issue.ObjectID)
		assert.Equal(t, "goal", issue.Kind)
		assert.Contains(t, issue.Recommendation, "Update ID prefix")
	}
}

func TestCycle141_ValidateYAML_EdgeCases(t *testing.T) {
	// 1. validateYAMLFile
	tmpDir := t.TempDir()
	missingFile := filepath.Join(tmpDir, "missing.yaml")
	err := validateYAMLFile(missingFile)
	require.Error(t, err)

	invalidFile := filepath.Join(tmpDir, "invalid.yaml")
	require.NoError(t, fileutil.WriteFile(invalidFile, []byte("bad: [syntax:\n"), paths.FilePerm644))
	err = validateYAMLFile(invalidFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "YAML syntax error")

	validFile := filepath.Join(tmpDir, "valid.yaml")
	require.NoError(t, fileutil.WriteFile(validFile, []byte("good: syntax\n"), paths.FilePerm644))
	require.NoError(t, validateYAMLFile(validFile))

	// 2. findZQKPath
	assert.Equal(t, "zqk", findZQKPath(tmpDir))
	zqkBin := filepath.Join(tmpDir, "zqk")
	require.NoError(t, fileutil.WriteFile(zqkBin, []byte("binary"), paths.FilePerm755))
	assert.Equal(t, zqkBin, findZQKPath(tmpDir))

	// 3. getRelativePath
	assert.Equal(t, "valid.yaml", getRelativePath(tmpDir, validFile))
	assert.NotEmpty(t, getRelativePath(tmpDir, "/other/path"))
}

func TestCycle141_ValidateYAML_RegisterHashEdgeCases(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})

	cliCtx := cli.ContextForProjectRoot(proj.Root)
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// 1. Non-existent file
	err := registerHashViaSystemCheck(cliCtx, logger, filepath.Join(proj.Root, "nonexistent.yaml"), true)
	require.Error(t, err)

	// 2. File without kind or ID returns nil
	noKindFile := filepath.Join(proj.Root, "no_kind.yaml")
	require.NoError(t, fileutil.WriteFile(noKindFile, []byte("name: test\nvalue: 123\n"), paths.FilePerm644))
	err = registerHashViaSystemCheck(cliCtx, logger, noKindFile, true)
	require.NoError(t, err)

	// 3. File with kind but no ID returns nil
	noIDFile := filepath.Join(proj.Root, "no_id.yaml")
	require.NoError(t, fileutil.WriteFile(noIDFile, []byte("kind: goal\ntitle: test\n"), paths.FilePerm644))
	err = registerHashViaSystemCheck(cliCtx, logger, noIDFile, true)
	require.NoError(t, err)

	// 4. Hash mismatch detection
	backlogDir := filepath.Join(proj.Root, paths.ProcessBacklogDir)
	require.NoError(t, fileutil.MkdirAll(backlogDir, paths.DirPerm755))
	bliFile := filepath.Join(backlogDir, "BLI-TAMPER-001.yaml")
	require.NoError(t, fileutil.WriteFile(bliFile, []byte("id: BLI-TAMPER-001\nkind: backlog_item\ntitle: Original\n"), paths.FilePerm644))

	// Write hash registry JSON with mismatched hash
	registryFile := filepath.Join(backlogDir, ".backlog_item.hashes")
	registryContent := []byte("{\"hashes\":{\"BLI-TAMPER-001.yaml\":\"0000000000000000000000000000000000000000000000000000000000000000\"}}")
	require.NoError(t, fileutil.WriteFile(registryFile, registryContent, paths.FilePerm644))

	err = registerHashViaSystemCheck(cliCtx, logger, bliFile, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hash mismatch detected")
}

func TestCycle141_ScenarioBuilder_CreateScenarioBuilderHelper(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	targetDir := filepath.Join(proj.Root, "test-target-sb")
	t.Cleanup(func() {
		storage.SetCacheOperationHandler(nil)
		cli.UnregisterStorageForProjectRoot(targetDir)
		storage.ShutdownHashRegistriesForProjectTesting(targetDir)
		cli.UnregisterStorageForProjectRoot(proj.Root)
		storage.ShutdownHashRegistriesForProjectTesting(proj.Root)
	})

	cmd := &cobra.Command{Use: "scenario-builder"}
	cmd.SetContext(pkgctx.NewSystemContext())

	sb, err := createScenarioBuilder(cmd, targetDir)
	require.NoError(t, err)
	require.NotNil(t, sb)
	assert.Equal(t, targetDir, sb.config.TargetDir)
	assert.NotNil(t, sb.storage)
	assert.NotNil(t, sb.coordinator)

	// Verify object ID cache has targetDir set
	cache := system.GetGlobalObjectIDCache()
	assert.NotNil(t, cache)
}

func TestCycle141_ScenarioBuilder_FindScenarioByName(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	sb := NewScenarioBuilder(proj.Root, proj.FileStorage, coordinator)
	sb.logger = logging.NewEventLogger(pkgctx.NewSystemContext())
	sb.secCtx = pkgctx.NewSystemSecurityContext()

	ctx := pkgctx.NewSystemContext()
	// Scenario not found returns error
	_, err := sb.findScenarioByName(ctx, "NONEXISTENT")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scenario not found")
}

func TestCycle141_ScenarioBuilder_GenerateSpecs(t *testing.T) {
	tmpDir := t.TempDir()
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	sb := &ScenarioBuilder{
		coordinator: coordinator,
	}

	// Existing file instead of dir causes EnsureOutputDir error
	blockerFile := filepath.Join(tmpDir, "file_as_dir")
	require.NoError(t, fileutil.WriteFile(blockerFile, []byte("data"), paths.FilePerm644))

	// Attempting to generate specs with file as directory returns error
	_, err := sb.generateSpecs(filepath.Join(blockerFile, "sub"))
	assert.Error(t, err)

	// Valid specs generation
	specsDir := filepath.Join(tmpDir, "specs_out")
	count, err := sb.generateSpecs(specsDir)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 0)
}

func TestCycle141_ValidateYAML_ProcessSingleYAMLFile(t *testing.T) {
	tmpDir := t.TempDir()
	cmd := &cobra.Command{Use: "validate-yaml"}
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

	cliCtx := cli.ContextForProjectRoot(tmpDir)
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	result := &ValidateYAMLResult{}

	// 1. Invalid YAML
	badFile := filepath.Join(tmpDir, "bad.yaml")
	require.NoError(t, fileutil.WriteFile(badFile, []byte("bad: [yaml\n"), paths.FilePerm644))
	flags := &ValidateYAMLFlags{Quiet: false, RegisterHashes: false}
	processSingleYAMLFile(cmd, badFile, cliCtx, logger, flags, result)
	assert.Equal(t, 1, result.InvalidCount)
	assert.Len(t, result.Errors, 1)

	// 2. Valid YAML with Quiet = false
	goodFile := filepath.Join(tmpDir, "good.yaml")
	require.NoError(t, fileutil.WriteFile(goodFile, []byte("good: yaml\n"), paths.FilePerm644))
	processSingleYAMLFile(cmd, goodFile, cliCtx, logger, flags, result)
	assert.Equal(t, 1, result.ValidCount)
	assert.Contains(t, buf.String(), "✓")
}

func TestCycle141_ScenarioBuilder_CopyConfigAndIDGeneration(t *testing.T) {
	// 1. validateCopyConfig
	emptyCfg := &ScenarioCopyConfig{ObjectIDs: nil}
	require.Error(t, validateCopyConfig(emptyCfg))
	validCfg := &ScenarioCopyConfig{ObjectIDs: []string{"GOAL-001"}}
	require.NoError(t, validateCopyConfig(validCfg))

	// 2. generateScenarioID
	sb := &ScenarioBuilder{config: &ScenarioBuilderConfig{}}
	idValidator := validation.GetIDValidator()
	require.NoError(t, idValidator.LoadPatterns())

	cfgWithOverride := &ScenarioCopyConfig{IDPrefixOverride: "MOCK-"}
	genID := sb.generateScenarioID("GOAL-001", "goal", cfgWithOverride, idValidator)
	assert.Equal(t, "MOCK-001", genID)

	cfgNoOverride := &ScenarioCopyConfig{}
	genID2 := sb.generateScenarioID("GOAL-001", "goal", cfgNoOverride, idValidator)
	assert.Contains(t, genID2, "TEST-")

	// 3. calculateTotalCopied
	counts := map[string]int{"goal": 3, "requirement": 7}
	assert.Equal(t, 10, calculateTotalCopied(counts))

	// 4. validateSourceDirectory
	tmpDir := t.TempDir()
	_, err := validateSourceDirectory(filepath.Join(tmpDir, "nonexistent"))
	assert.Error(t, err)

	filePath := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, fileutil.WriteFile(filePath, []byte("data"), paths.FilePerm644))
	_, err = validateSourceDirectory(filePath)
	assert.Error(t, err)

	info, err := validateSourceDirectory(tmpDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// 5. shouldSkipFile
	assert.True(t, shouldSkipFile(info, "._file.yaml"))
	assert.False(t, shouldSkipFile(info, "normal.yaml"))
}

func TestCycle141_ScenarioBuilder_CreateHelpersAndScenarioLifecycle(t *testing.T) {
	initStatus := getInitialStatusForScenario()
	assert.NotEmpty(t, initStatus)

	sb := &ScenarioBuilder{config: &ScenarioBuilderConfig{}}
	obj := sb.buildNewScenarioObject("TestScen", "SCE-001", initStatus)
	assert.Equal(t, scenarioBuilderKindScenario, obj[objects.FieldKeyKind])
	assert.Equal(t, initStatus, obj[objects.FieldKeyStatus])
}

func TestCycle141_ScenarioBuilder_PIIAndMetrics(t *testing.T) {
	// 1. identifyPIIFieldsFromSpec
	emptyPII := identifyPIIFieldsFromSpec(nil)
	assert.Empty(t, emptyPII)

	spec := &objects.Spec{
		Fields: map[string]any{
			"email": map[string]any{
				"checklist": map[string]any{
					"security": "sensitive pii data",
				},
			},
			"normal_field": map[string]any{
				"checklist": map[string]any{
					"security": "public unclassified",
				},
			},
		},
	}
	piiFields := identifyPIIFieldsFromSpec(spec)
	assert.True(t, piiFields["email"])
	assert.False(t, piiFields["normal_field"])

	// 2. getCommonPIIPatterns
	patterns := getCommonPIIPatterns()
	assert.Contains(t, patterns, "email")
	assert.Contains(t, patterns, "ssn")

	// 3. Metrics functions
	builder := NewMetricBuilder("test_op")
	assert.NotNil(t, builder)

	noOp := GetNoOpMetricRecorder()
	assert.NotNil(t, noOp)

	factory := NewMetricRecorderFactory(true)
	assert.NotNil(t, factory)
}

func TestCycle141_ScenarioBuilder_EventSubscriber_Profiles(t *testing.T) {
	profiles := []string{
		ScenarioBuilderProfileName,
		string(pkgctx.ProfileMCP),
		string(pkgctx.ProfileSystem),
		string(pkgctx.ProfileAIAgent),
		string(pkgctx.ProfileDebug),
		string(pkgctx.ProfileHuman),
		"custom_profile_xyz",
	}

	for _, p := range profiles {
		sub := NewScenarioBuilderEventSubscriber(p)
		require.NotNil(t, sub)
		assert.Equal(t, p, sub.profile)
	}
}

func TestCycle141_FixHashes_CollectFilesByKind(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// 1. Unknown kind returns error
	_, errs := collectFilesByKind(proj.Root, "unknown_kind_xyz", logger, true)
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0], "unknown object kind")

	// 2. Kind with missing dir returns empty files without error
	files, errs := collectFilesByKind(proj.Root, "backlog_item", logger, false)
	assert.Empty(t, files)
	assert.Empty(t, errs)

	// 3. Kind with files skips dot-files
	goalDir := filepath.Join(proj.Root, paths.ProcessGoalsDir)
	require.NoError(t, fileutil.MkdirAll(goalDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(goalDir, "GOAL-001.yaml"), []byte("id: GOAL-001\n"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(goalDir, ".hash_registry"), []byte("hash"), paths.FilePerm644))

	files, errs = collectFilesByKind(proj.Root, "goal", logger, true)
	assert.Len(t, files, 1)
	assert.Empty(t, errs)
	assert.Contains(t, files[0], "GOAL-001.yaml")

	// 4. collectInternalFiles
	intFiles, intErrs := collectInternalFiles(proj.Root, logger, true)
	assert.Empty(t, intFiles)
	assert.Empty(t, intErrs)
}

func TestCycle141_FixRegistration_ApplyFixFullCycle(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})

	configDir := filepath.Join(proj.Root, paths.ProcessInternalConfigsDir)
	require.NoError(t, fileutil.MkdirAll(configDir, paths.DirPerm755))
	configPath := filepath.Join(configDir, paths.IdPrefixesConfigFile)
	initialConfig := "kind_to_prefixes:\n  goal:\n    - GOAL-\n"
	require.NoError(t, fileutil.WriteFile(configPath, []byte(initialConfig), paths.FilePerm644))

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	cmd := &cobra.Command{}
	issues := []registrationIssue{
		{ObjectID: "CUSTOM-001", Kind: "goal"},
		{ObjectID: "CUSTOM-002", Kind: "goal"},
		{ObjectID: "TASK-001", Kind: "backlog_item"},
	}

	// 1. Dry run does not write
	addedDry, err := applyRegistrationFix(cmd, proj.Root, issues, true, true, logger)
	require.NoError(t, err)
	assert.Len(t, addedDry, 2)

	// 2. Real apply writes updated config
	added, err := applyRegistrationFix(cmd, proj.Root, issues, false, false, logger)
	require.NoError(t, err)
	assert.Len(t, added, 2)

	// Verify file content updated
	updatedData, err := fileutil.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(updatedData), "CUSTOM-")
	assert.Contains(t, string(updatedData), "TASK-")

	// 3. ensurePrefixForKind returns false for existing
	cfg := &validation.IDPrefixesConfig{KindToPrefixes: map[string][]string{"goal": {"GOAL-"}}}
	assert.True(t, ensurePrefixForKind(cfg, "goal", "NEW-"))
	assert.False(t, ensurePrefixForKind(cfg, "goal", "NEW-"))
}

func TestCycle141_ScenarioBuilder_CopyBootstrapConfigFiles_Execution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})

	specsDir := filepath.Join(proj.Root, paths.ProcessInternalDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "id_prefixes_config.yaml"), []byte("test: true\n"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "kind_mappings_config.yaml"), []byte("test: true\n"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "namespaces_config.yaml"), []byte("test: true\n"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "paths_config.yaml"), []byte("test: true\n"), paths.FilePerm644))

	targetScenarioDir := filepath.Join(proj.Root, "scenario-sandbox")
	sb := newTestScenarioBuilder(t, targetScenarioDir)
	sb.projectRoot = proj.Root

	targetInternal := filepath.Join(targetScenarioDir, paths.ProcessInternalDir)
	require.NoError(t, fileutil.MkdirAll(targetInternal, paths.DirPerm755))

	origWd, err := fileutil.Getwd()
	require.NoError(t, err)
	defer func() {
		require.NoError(t, fileutil.Chdir(origWd))
	}()
	require.NoError(t, fileutil.Chdir(targetScenarioDir))

	err = sb.copyBootstrapConfigFiles(targetInternal)
	require.NoError(t, err)

	copiedPath := filepath.Join(targetInternal, "id_prefixes_config.yaml")
	assert.True(t, fileutil.Exists(copiedPath))
}

func TestCycle141_ScenarioBuilder_FlagsAndDirResolution(t *testing.T) {
	// 1. resolveTargetDirectory
	absDir := "/tmp/absolute/scenario"
	assert.Equal(t, absDir, resolveTargetDirectory(absDir))

	relDir := "relative/scenario"
	resolved := resolveTargetDirectory(relDir)
	assert.True(t, filepath.IsAbs(resolved))
	assert.Contains(t, resolved, "relative/scenario")

	// 2. validateTargetDirectory
	assert.NoError(t, validateTargetDirectory("/tmp/any/dir"))
}

func TestCycle141_ScenarioBuilder_DataLoader_References_Unit(t *testing.T) {
	idStream := map[string]string{
		"GOAL-PLACEHOLDER": "GOAL-ACTUAL-001",
	}

	// 1. resolveReference
	assert.Equal(t, "GOAL-ACTUAL-001", resolveReference("GOAL-PLACEHOLDER", idStream))
	assert.Equal(t, "UNKNOWN-REF", resolveReference("UNKNOWN-REF", idStream))

	// 2. resolveReferenceList
	refs := []any{"GOAL-PLACEHOLDER", "OTHER-REF", 12345}
	resolvedList, hasResolved := resolveReferenceList(refs, idStream)
	assert.True(t, hasResolved)
	assert.Equal(t, "GOAL-ACTUAL-001", resolvedList[0])
	assert.Equal(t, "OTHER-REF", resolvedList[1])
	assert.Equal(t, 12345, resolvedList[2])

	// 3. processSingleReferenceField
	obj := map[string]any{"goal_ref": "GOAL-PLACEHOLDER", "other_field": "val"}
	assert.True(t, processSingleReferenceField(obj, "goal_ref", "GOAL-PLACEHOLDER", idStream))
	assert.Equal(t, "GOAL-ACTUAL-001", obj["goal_ref"])
	assert.False(t, processSingleReferenceField(obj, "other_field", "val", idStream))

	// 4. processReferenceListField
	objList := map[string]any{"goal_refs": []any{"GOAL-PLACEHOLDER"}}
	assert.True(t, processReferenceListField(objList, "goal_refs", objList["goal_refs"], idStream))
	assert.False(t, processReferenceListField(objList, "not_refs", "abc", idStream))

	// 5. collectReferenceStrings
	assert.Len(t, collectReferenceStrings([]any{"REF-1", "REF-2", 42}), 2)
	assert.Len(t, collectReferenceStrings([]string{"REF-3", "REF-4"}), 2)
	assert.Empty(t, collectReferenceStrings(999))
}

func TestCycle141_ScenarioBuilder_Build_EdgeCases(t *testing.T) {
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	sb := &ScenarioBuilder{
		config:      &ScenarioBuilderConfig{},
		coordinator: coordinator,
	}
	ctx := pkgctx.NewSystemContext()

	// 1. Build with no kinds returns error
	err := sb.Build(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no object kinds specified")

	// 2. BuildFromObjects with invalid directory returns error
	tmpDir := t.TempDir()
	blockerFile := filepath.Join(tmpDir, "file_as_dir")
	require.NoError(t, fileutil.WriteFile(blockerFile, []byte("data"), paths.FilePerm644))

	sb.copyConfig = &ScenarioCopyConfig{
		TargetDir: filepath.Join(blockerFile, "nested"),
		ObjectIDs: []string{"GOAL-001"},
	}
	err = sb.BuildFromObjects(ctx)
	assert.Error(t, err)
}

func TestCycle141_ScenarioBuilder_Handlers_DryRunAndConfigure(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Cleanup(func() {
		storage.SetCacheOperationHandler(nil)
		cli.UnregisterStorageForProjectRoot(proj.Root)
		storage.ShutdownHashRegistriesForProjectTesting(proj.Root)
	})

	cmd := &cobra.Command{Use: "scenario-builder"}
	cmd.SetContext(pkgctx.NewSystemContext())
	sb, err := createScenarioBuilder(cmd, proj.Root)
	require.NoError(t, err)
	sb.secCtx = pkgctx.NewSystemSecurityContext()

	// 1. configureBuilderFromFlags error when kinds empty
	err = configureBuilderFromFlags(sb, &ScenarioBuilderFlags{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--kinds is required")

	// 2. configureBuilderFromFlags success
	flags := &ScenarioBuilderFlags{
		Kinds:            []string{"goal", "requirement"},
		DefaultCount:     3,
		DefaultDiversity: 2,
		LinkProb:         0.5,
		Counts:           map[string]int{"goal": 5},
		Diversity:        map[string]int{"goal": 3},
	}
	require.NoError(t, configureBuilderFromFlags(sb, flags))
	assert.Equal(t, 5, sb.config.Counts["goal"])
	assert.Equal(t, 3, sb.config.Diversity["goal"])

	// 3. handleDryRun
	require.NoError(t, handleDryRun(sb, []string{"goal"}))

	// 4. handleCopyObjects dry-run
	copyFlags := &ScenarioBuilderFlags{
		SourceProject: proj.Root,
		TargetDir:     filepath.Join(proj.Root, "target_scenario"),
		ObjectIDs:     []string{"GOAL-001", "REQ-001"},
		DryRun:        true,
	}
	require.NoError(t, handleCopyObjects(cmd, sb, copyFlags))

	// 5. handleLoadFromScenario failure
	err = handleLoadFromScenario(sb, "NONEXISTENT-SCENARIO", proj.Root)
	assert.Error(t, err)
}

func TestCycle141_CleanObjectForUpdate(t *testing.T) {
	input := map[string]any{
		objects.FieldKeyID:          "GOAL-123",
		objects.FieldKeyKind:        "goal",
		objects.FieldKeyCreatedAt:   "2026-01-01T00:00:00Z",
		objects.FieldKeyCreatedBy:   "user1",
		objects.FieldKeyUpdatedAt:   "2026-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:   "user2",
		objects.FieldKeyTitle:       "Retained Title",
		objects.FieldKeyDescription: "Retained Description",
		"custom_field":              42,
	}

	cleaned := cleanObjectForUpdate(input)
	assert.NotContains(t, cleaned, objects.FieldKeyID)
	assert.NotContains(t, cleaned, objects.FieldKeyKind)
	assert.NotContains(t, cleaned, objects.FieldKeyCreatedAt)
	assert.NotContains(t, cleaned, objects.FieldKeyCreatedBy)
	assert.NotContains(t, cleaned, objects.FieldKeyUpdatedAt)
	assert.NotContains(t, cleaned, objects.FieldKeyUpdatedBy)

	assert.Equal(t, "Retained Title", cleaned[objects.FieldKeyTitle])
	assert.Equal(t, "Retained Description", cleaned[objects.FieldKeyDescription])
	assert.Equal(t, 42, cleaned["custom_field"])
}

func TestCycle141_TrackValidationScope_NormalAndPanicRecovery(t *testing.T) {
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	sb := &ScenarioBuilder{coordinator: coordinator}
	ctx := pkgctx.NewSystemContext()

	var counter int
	var counterMu sync.Mutex
	var wg sync.WaitGroup
	state := &layerProcessingState{
		activeValidations:   &wg,
		validationCounter:   &counter,
		validationCounterMu: &counterMu,
	}

	doneFn := sb.trackValidationScope(ctx, state, "goal", "GOAL-001", 1)
	assert.Equal(t, 1, counter)
	doneFn()
	assert.Equal(t, 0, counter)

	var counter2 int
	var counterMu2 sync.Mutex
	var wg2 sync.WaitGroup
	state2 := &layerProcessingState{
		activeValidations:   &wg2,
		validationCounter:   &counter2,
		validationCounterMu: &counterMu2,
	}

	doneFn2 := sb.trackValidationScope(ctx, state2, "goal", "GOAL-002", -1)
	wg2.Done()
	assert.NotPanics(t, func() {
		doneFn2()
	})
}

func TestCycle141_ScenarioBuilder_CopyFile_Branches(t *testing.T) {
	sb := &ScenarioBuilder{config: &ScenarioBuilderConfig{}}
	tmpDir := t.TempDir()

	sourceFile := filepath.Join(tmpDir, "source.txt")
	require.NoError(t, fileutil.WriteFile(sourceFile, []byte("payload"), paths.FilePerm644))

	err := sb.copyFile(filepath.Join(tmpDir, "nonexistent.txt"), filepath.Join(tmpDir, "target.txt"))
	require.Error(t, err)

	targetFile := filepath.Join(tmpDir, "target.txt")
	require.NoError(t, sb.copyFile(sourceFile, targetFile))
	content, err := fileutil.ReadFile(targetFile)
	require.NoError(t, err)
	assert.Equal(t, "payload", string(content))

	blockerFile := filepath.Join(tmpDir, "blocker")
	require.NoError(t, fileutil.WriteFile(blockerFile, []byte("data"), paths.FilePerm644))
	err = sb.copyFile(sourceFile, filepath.Join(blockerFile, "nested", "target.txt"))
	require.Error(t, err)

	targetDir := filepath.Join(tmpDir, "dir_as_target")
	require.NoError(t, fileutil.MkdirAll(targetDir, paths.DirPerm755))
	err = sb.copyFile(sourceFile, targetDir)
	require.Error(t, err)
}

func TestCycle141_ScenarioBuilder_CopyDirectory_Branches(t *testing.T) {
	sb := &ScenarioBuilder{config: &ScenarioBuilderConfig{}}
	tmpDir := t.TempDir()

	err := sb.copyDirectory(filepath.Join(tmpDir, "nonexistent"), filepath.Join(tmpDir, "dest"))
	require.Error(t, err)

	sourceFile := filepath.Join(tmpDir, "source_file.txt")
	require.NoError(t, fileutil.WriteFile(sourceFile, []byte("content"), paths.FilePerm644))
	err = sb.copyDirectory(sourceFile, filepath.Join(tmpDir, "dest"))
	require.Error(t, err)

	sourceDir := filepath.Join(tmpDir, "src_dir")
	subDir := filepath.Join(sourceDir, "sub")
	require.NoError(t, fileutil.MkdirAll(subDir, paths.DirPerm755))

	f1 := filepath.Join(sourceDir, "file1.txt")
	f2 := filepath.Join(subDir, "file2.yaml")
	skipFile := filepath.Join(sourceDir, "._file1.txt")
	require.NoError(t, fileutil.WriteFile(f1, []byte("f1"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(f2, []byte("f2"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(skipFile, []byte("apple"), paths.FilePerm644))

	targetDir := filepath.Join(tmpDir, "target_dir")
	require.NoError(t, sb.copyDirectory(sourceDir, targetDir))
	assert.True(t, fileutil.Exists(filepath.Join(targetDir, "file1.txt")))
	assert.True(t, fileutil.Exists(filepath.Join(targetDir, "sub", "file2.yaml")))
	assert.False(t, fileutil.Exists(filepath.Join(targetDir, "._file1.txt")))

	err = sb.copyDirectory(sourceDir, filepath.Join(sourceFile, "nested"))
	require.Error(t, err)
}

func TestCycle141_ScenarioBuilder_ProfileAndConfigFile_Branches(t *testing.T) {
	sb := &ScenarioBuilder{config: &ScenarioBuilderConfig{}}
	tmpDir := t.TempDir()

	destDir := filepath.Join(tmpDir, "dest_profile")
	require.NoError(t, sb.copyScenarioBuilderProfile(destDir))
	profilePath := filepath.Join(destDir, "pkg", "cli", "profiles", ScenarioBuilderProfileName+".yaml")
	assert.True(t, fileutil.Exists(profilePath))

	blockerFile := filepath.Join(tmpDir, "blocker.txt")
	require.NoError(t, fileutil.WriteFile(blockerFile, []byte("data"), paths.FilePerm644))
	err := sb.copyScenarioBuilderProfile(blockerFile)
	require.Error(t, err)

	err = sb.copyConfigFile(filepath.Join(tmpDir, "no_such_config.yaml"), filepath.Join(tmpDir, "out.yaml"))
	require.Error(t, err)

	cfgSource := filepath.Join(tmpDir, "source_config.yaml")
	require.NoError(t, fileutil.WriteFile(cfgSource, []byte("config: true\n"), paths.FilePerm644))
	cfgTarget := filepath.Join(tmpDir, "copied_config", "out.yaml")
	require.NoError(t, sb.copyConfigFile(cfgSource, cfgTarget))
	assert.True(t, fileutil.Exists(cfgTarget))

	err = sb.copyConfigFile(cfgSource, filepath.Join(blockerFile, "cannot_create", "out.yaml"))
	require.Error(t, err)
}

func TestCycle141_FixHashes_UtilityHelpers_ContextAndCandidates(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cli.SetContext(cmd, nil)
	_, _, _, err := initializeUtilityCommandContext(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get context")

	tmpDir := t.TempDir()
	cliCtx := cli.ContextForProjectRoot(tmpDir)
	cliCtx.ProjectRoot = ""
	cli.SetContext(cmd, cliCtx)
	_, _, _, err = initializeUtilityCommandContext(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a ZQK project")

	cliCtx.ProjectRoot = tmpDir
	cli.SetContext(cmd, cliCtx)
	ctx, logger, root, err := initializeUtilityCommandContext(cmd)
	require.NoError(t, err)
	assert.NotNil(t, ctx)
	assert.NotNil(t, logger)
	assert.Equal(t, tmpDir, root)

	assert.True(t, shouldSkipFileWalkCandidate("test.yaml", nil, assert.AnError))
	stat, err := os.Stat(tmpDir)
	require.NoError(t, err)
	assert.True(t, shouldSkipFileWalkCandidate(tmpDir, stat, nil))

	fPath := filepath.Join(tmpDir, "file.yaml")
	require.NoError(t, fileutil.WriteFile(fPath, []byte("x"), paths.FilePerm644))
	fStat, err := os.Stat(fPath)
	require.NoError(t, err)
	assert.False(t, shouldSkipFileWalkCandidate(fPath, fStat, nil))
	assert.True(t, shouldSkipFileWalkCandidate("/path/._apple.yaml", fStat, nil))
}

func TestCycle141_FixHashes_CollectFiles_And_ValidateFlags(t *testing.T) {
	tmpDir := t.TempDir()

	flags := &FixHashesFlags{Kind: "goal", Internal: true}
	err := validateFixHashesFlags(flags, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot use --kind and --internal together")

	flags = &FixHashesFlags{Kind: "goal"}
	err = validateFixHashesFlags(flags, []string{"path1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot specify paths when using --kind")

	flags = &FixHashesFlags{Internal: true}
	err = validateFixHashesFlags(flags, []string{"path1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot specify paths when using --internal")

	flags = &FixHashesFlags{Recursive: true}
	require.NoError(t, validateFixHashesFlags(flags, []string{"path1"}))

	files, errs := collectFilesFromArgs([]string{filepath.Join(tmpDir, "missing")}, false, false)
	assert.Empty(t, files)
	assert.Len(t, errs, 1)

	subDir := filepath.Join(tmpDir, "yamldir")
	require.NoError(t, fileutil.MkdirAll(subDir, paths.DirPerm755))
	f1 := filepath.Join(subDir, "a.yaml")
	f2 := filepath.Join(subDir, "b.yml")
	f3 := filepath.Join(subDir, "c.txt")
	require.NoError(t, fileutil.WriteFile(f1, []byte("k: v"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(f2, []byte("k: v"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(f3, []byte("text"), paths.FilePerm644))

	files, errs = collectFilesFromArgs([]string{subDir}, false, false)
	assert.Empty(t, files)
	assert.Contains(t, errs[0], "use --recursive")

	files, errs = collectFilesFromArgs([]string{subDir}, true, false)
	assert.Len(t, files, 2)
	assert.Empty(t, errs)

	files, errs = collectFilesFromArgs([]string{f3}, false, false)
	assert.Len(t, files, 1)
	assert.Contains(t, errs[0], "does not have .yaml or .yml")

	files, errs = collectFilesFromArgs([]string{f3}, false, true)
	assert.Len(t, files, 1)
	assert.Empty(t, errs)
}

func TestCycle141_FixHashes_ShowProgress_And_OutputSummary(t *testing.T) {
	cmd := &cobra.Command{Use: "fix-hashes"}
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

	showProgress(cmd, true, 0, 50)
	assert.Empty(t, buf.String())

	showProgress(cmd, false, 0, 5)
	assert.Empty(t, buf.String())

	showProgress(cmd, false, 0, 20)
	assert.Contains(t, buf.String(), "Processing 20 objects...")
	buf.Reset()

	showProgress(cmd, false, 100, 200)
	assert.Contains(t, buf.String(), "Processing 100/200 objects...")
	buf.Reset()

	res := &FixHashesResult{FixedCount: 3, SkippedCount: 1, ErrorCount: 0, TotalFiles: 4}
	outputSummary(cmd, res)
	assert.Contains(t, buf.String(), "Hash fix summary: 3 fixed, 1 skipped")
	assert.Contains(t, buf.String(), "All hashes fixed successfully")
	buf.Reset()

	resDry := &FixHashesResult{
		FixedCount: 2, SkippedCount: 0, ErrorCount: 1, TotalFiles: 3, DryRun: true,
		Errors: []string{"Corrupt hash on file X"},
	}
	outputSummary(cmd, resDry)
	assert.Contains(t, buf.String(), "would be fixed")
	assert.Contains(t, buf.String(), "Dry run: no changes made")
	assert.Contains(t, buf.String(), "Corrupt hash on file X")
}

func TestCycle141_ScenarioBuilder_LoadHelpers_TypeConversions(t *testing.T) {
	val, ok := convertToInt(42)
	assert.True(t, ok)
	assert.Equal(t, 42, val)

	val, ok = convertToInt(float64(84))
	assert.True(t, ok)
	assert.Equal(t, 84, val)

	val, ok = convertToInt("invalid")
	assert.False(t, ok)
	assert.Equal(t, 0, val)

	kinds := loadKindsFromConfig(map[string]any{
		"kinds": []any{"goal", "requirement", 123},
	})
	assert.Equal(t, []string{"goal", "requirement", ""}, kinds)
	assert.Nil(t, loadKindsFromConfig(map[string]any{}))

	counts := loadCountsFromConfig(map[string]any{
		"counts": map[string]any{"goal": 5, "req": float64(10), "bad": "invalid"},
	})
	assert.Equal(t, 5, counts["goal"])
	assert.Equal(t, 10, counts["req"])
	assert.Nil(t, loadCountsFromConfig(map[string]any{}))

	div := loadDiversityFromConfig(map[string]any{
		"diversity": map[string]any{"goal": 2, "req": float64(4)},
	})
	assert.Equal(t, 2, div["goal"])
	assert.Equal(t, 4, div["req"])
	assert.Nil(t, loadDiversityFromConfig(map[string]any{}))
}

func TestCycle141_ScenarioBuilder_LoadHelpers_ConfigApplication(t *testing.T) {
	cfgMap := map[string]any{
		"default_count":     12,
		"default_diversity": float64(6),
		"link_probability":  0.85,
		"time_range":        "72h",
		"start_time":        "2026-10-23T15:00:00Z",
	}

	assert.Equal(t, 12, loadDefaultCountFromConfig(cfgMap))
	assert.Equal(t, 6, loadDefaultDiversityFromConfig(cfgMap))
	assert.Equal(t, 0.85, loadLinkProbabilityFromConfig(cfgMap))
	assert.Equal(t, 72*time.Hour, loadTimeRangeFromConfig(cfgMap))
	expectedTime, err := time.Parse(time.RFC3339, "2026-10-23T15:00:00Z")
	require.NoError(t, err)
	assert.Equal(t, expectedTime, loadStartTimeFromConfig(cfgMap))

	assert.Equal(t, time.Duration(0), loadTimeRangeFromConfig(map[string]any{"time_range": "bad"}))
	assert.Equal(t, time.Time{}, loadStartTimeFromConfig(map[string]any{"start_time": "bad"}))

	sb := &ScenarioBuilder{config: &ScenarioBuilderConfig{}}
	applyConfigToBuilder(sb, cfgMap)
	assert.Equal(t, 12, sb.config.DefaultCount)
	assert.Equal(t, 6, sb.config.DefaultDiversity)
	assert.Equal(t, 0.85, sb.config.LinkProbability)
	assert.Equal(t, 72*time.Hour, sb.config.TimeRange)
	assert.Equal(t, expectedTime, sb.config.StartTime)

	_, err = extractDataGenerationConfig(map[string]any{}, "SCEN-01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not have data_generation_config")

	_, err = extractDataGenerationConfig(map[string]any{
		objects.FieldKeyDataGenerationConfig: "string_not_map",
	}, "SCEN-01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an object")

	extracted, err := extractDataGenerationConfig(map[string]any{
		objects.FieldKeyDataGenerationConfig: cfgMap,
	}, "SCEN-01")
	require.NoError(t, err)
	assert.NotNil(t, extracted)
}

func TestCycle141_FixRegistration_Helpers(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Cleanup(func() {
		storage.SetCacheOperationHandler(nil)
		cli.UnregisterStorageForProjectRoot(proj.Root)
		storage.ShutdownHashRegistriesForProjectTesting(proj.Root)
	})

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileHuman))
	val := validation.NewIDValidator(proj.Root)

	assert.Equal(t, "GOAL-", getPrefixFromID("GOAL-1234567890-abcdef12"))
	assert.Equal(t, "", getPrefixFromID("UNPREFIXED"))

	issue := checkRegistrationIssue(proj.Root, "NONEXISTENT-001", val, logger)
	assert.Nil(t, issue)

	corruptDir := filepath.Join(proj.Root, ".zqk", "process", "goals")
	require.NoError(t, fileutil.MkdirAll(corruptDir, paths.DirPerm755))
	corruptFile := filepath.Join(corruptDir, "GOAL-corrupt.yaml")
	require.NoError(t, fileutil.WriteFile(corruptFile, []byte(":::invalid yaml:::"), paths.FilePerm644))
	issue = checkRegistrationIssue(proj.Root, "GOAL-corrupt", val, logger)
	assert.Nil(t, issue)

	badKindFile := filepath.Join(corruptDir, "GOAL-badkind.yaml")
	badKindYAML := "id: GOAL-badkind\nkind: account\nusername: user1\n"
	require.NoError(t, fileutil.WriteFile(badKindFile, []byte(badKindYAML), paths.FilePerm644))
	issue = checkRegistrationIssue(proj.Root, "GOAL-badkind", val, logger)
	require.NotNil(t, issue)
	assert.Equal(t, "account", issue.Kind)
	assert.Contains(t, issue.Message, "does not match kind account")

	issues := findKindRegistrationIssues(proj.Root, "invalid_kind_dir", val, logger)
	assert.Empty(t, issues)

	issues = findKindRegistrationIssues(proj.Root, "goal", val, logger)
	assert.Len(t, issues, 1)
	assert.Equal(t, "GOAL-badkind", issues[0].ObjectID)
}

func TestCycle141_ScenarioBuilder_ProcessObjectsSequentially_AllBranches(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	t.Cleanup(func() {
		storage.SetCacheOperationHandler(nil)
		cli.UnregisterStorageForProjectRoot(proj.Root)
		storage.ShutdownHashRegistriesForProjectTesting(proj.Root)
	})

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	sb := &ScenarioBuilder{
		projectRoot: proj.Root,
		storage:     proj.FileStorage,
		coordinator: coordinator,
		secCtx:      pkgctx.NewSystemSecurityContext(),
		config:      &ScenarioBuilderConfig{TargetDir: proj.Root},
		force:       true,
		generated:   make(map[string]int),
	}

	var counter int
	var counterMu sync.Mutex
	var wg sync.WaitGroup
	state := &layerProcessingState{
		referenceCache:      make(map[string]bool),
		referenceCacheMu:    &sync.Mutex{},
		idStream:            make(map[string]string),
		idStreamMu:          &sync.Mutex{},
		activeValidations:   &wg,
		validationCounter:   &counter,
		validationCounterMu: &counterMu,
	}

	// 1. Missing referenced objects branch
	sb.dataFileObjects = []map[string]any{
		{
			objects.FieldKeyKind: "requirement",
			objects.FieldKeyID:   "REQ-001",
			"goal_refs":          []any{"GOAL-MISSING"},
		},
	}
	ctx := pkgctx.NewSystemContext()
	created, skipped := sb.processObjectsSequentially(ctx, state)
	assert.Equal(t, 0, created)
	assert.Equal(t, 1, skipped)

	// 2. Validation failure branch
	sb.dataFileObjects = []map[string]any{
		{},
	}
	created, skipped = sb.processObjectsSequentially(ctx, state)
	assert.Equal(t, 0, created)
	assert.Equal(t, 1, skipped)
}

func TestCycle141_ScenarioBuilder_PipelineStages_Direct(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	t.Cleanup(func() {
		storage.SetCacheOperationHandler(nil)
		cli.UnregisterStorageForProjectRoot(proj.Root)
		storage.ShutdownHashRegistriesForProjectTesting(proj.Root)
	})

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	sb := &ScenarioBuilder{
		projectRoot: proj.Root,
		coordinator: coordinator,
		config:      &ScenarioBuilderConfig{TargetDir: proj.Root},
		dataFileObjects: []map[string]any{
			{
				objects.FieldKeyID:       "ACC-USER01",
				objects.FieldKeyKind:     objects.KindAccount,
				objects.FieldKeyUsername: "admin",
			},
		},
	}

	payload := &scenarioBuilderPayload{
		sb:                sb,
		ctx:               pkgctx.NewSystemContext(),
		referenceCache:    make(map[string]bool),
		referenceCacheMu:  &sync.Mutex{},
		idStream:          make(map[string]string),
		idStreamMu:        &sync.Mutex{},
		activeValidations: &sync.WaitGroup{},
	}

	// 1. stageScenarioPrepareCache direct execution
	res, err := stageScenarioPrepareCache(nil, payload)
	require.NoError(t, err)
	assert.NotNil(t, res)

	checker := storage.GetCacheChecker()
	require.NotNil(t, checker)
	_, _ = checker("ACC-USER01")
	_, _ = checker("account:admin")
	_, _ = checker("NONEXISTENT")

	// 2. stageScenarioSetupInfrastructure error when TargetDir is blocked
	blocker := filepath.Join(proj.Root, "blocker_file")
	require.NoError(t, fileutil.WriteFile(blocker, []byte("data"), paths.FilePerm644))
	sb.config.TargetDir = filepath.Join(blocker, "sub")
	_, err = stageScenarioSetupInfrastructure(nil, payload)
	require.Error(t, err)
}

func TestCycle141_ScenarioBuilder_CopyBootstrapConfigFiles_FallbackSearch(t *testing.T) {
	emptyDir := t.TempDir()
	sb := newTestScenarioBuilder(t, emptyDir)
	sb.projectRoot = emptyDir

	targetInternal := filepath.Join(emptyDir, "target_internal")
	require.NoError(t, fileutil.MkdirAll(targetInternal, paths.DirPerm755))

	origWd, err := fileutil.Getwd()
	require.NoError(t, err)
	defer func() {
		require.NoError(t, fileutil.Chdir(origWd))
	}()
	require.NoError(t, fileutil.Chdir(emptyDir))

	err = sb.copyBootstrapConfigFiles(targetInternal)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot find source bootstrap directory")
}

func TestCycle141_ScenarioBuilder_CreateObjectInLayer_Direct(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	t.Cleanup(func() {
		storage.SetCacheOperationHandler(nil)
		cli.UnregisterStorageForProjectRoot(proj.Root)
		storage.ShutdownHashRegistriesForProjectTesting(proj.Root)
	})

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	sb := &ScenarioBuilder{
		projectRoot: proj.Root,
		storage:     proj.FileStorage,
		coordinator: coordinator,
		secCtx:      pkgctx.NewSystemSecurityContext(),
		config:      &ScenarioBuilderConfig{TargetDir: proj.Root},
		force:       true,
		generated:   make(map[string]int),
	}

	var counter int
	var counterMu sync.Mutex
	var wg sync.WaitGroup
	state := &layerProcessingState{
		referenceCache:      make(map[string]bool),
		referenceCacheMu:    &sync.Mutex{},
		idStream:            make(map[string]string),
		idStreamMu:          &sync.Mutex{},
		activeValidations:   &wg,
		validationCounter:   &counter,
		validationCounterMu: &counterMu,
	}

	ctx := pkgctx.NewSystemContext()
	var createdMu, skippedMu sync.Mutex
	created := 0

	objValid := map[string]any{
		objects.FieldKeyKind:  "goal",
		objects.FieldKeyTitle: "Layer Goal 1",
	}
	err := sb.createObjectInLayer(ctx, objValid, 0, 0, state, &createdMu, &skippedMu, &created)
	require.NoError(t, err)
	assert.Equal(t, 1, created)

	objInvalid := map[string]any{}
	err = sb.createObjectInLayer(ctx, objInvalid, 1, 0, state, &createdMu, &skippedMu, &created)
	require.Error(t, err)
}

func TestCycle141_ScenarioBuilder_LifecycleObjects_Branches(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	t.Cleanup(func() {
		storage.SetCacheOperationHandler(nil)
		cli.UnregisterStorageForProjectRoot(proj.Root)
		storage.ShutdownHashRegistriesForProjectTesting(proj.Root)
	})

	sb := &ScenarioBuilder{
		projectRoot: proj.Root,
		storage:     proj.FileStorage,
		secCtx:      pkgctx.NewSystemSecurityContext(),
		config:      &ScenarioBuilderConfig{TargetDir: proj.Root},
	}
	ctx := pkgctx.NewSystemContext()

	sb.config.TargetDir = t.TempDir()
	require.NoError(t, sb.createLifecycleObjects(ctx))

	blockerFile := filepath.Join(sb.config.TargetDir, paths.ProcessInternalLifecyclesDir)
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(blockerFile), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(blockerFile, []byte("data"), paths.FilePerm644))
	err := sb.createLifecycleObjects(ctx)
	require.Error(t, err)

	err = sb.createLifecycleObjectFromFile(ctx, filepath.Join(t.TempDir(), "missing.yaml"), "goal", "001")
	require.Error(t, err)
}

func TestCycle141_ScenarioBuilder_CreateScenarioBuilder_CacheHandlerAndDirErrors(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Cleanup(func() {
		storage.SetCacheOperationHandler(nil)
		cli.UnregisterStorageForProjectRoot(proj.Root)
		storage.ShutdownHashRegistriesForProjectTesting(proj.Root)
	})

	cmd := &cobra.Command{Use: "scenario-builder"}
	cliCtx := cli.ContextForProjectRoot(proj.Root).WithProfile("human")
	cli.SetContext(cmd, cliCtx)
	cmd.SetContext(pkgctx.NewSystemContext())

	sb, err := createScenarioBuilder(cmd, proj.Root)
	require.NoError(t, err)
	assert.NotNil(t, sb)

	handler := storage.GetCacheOperationHandler()
	require.NotNil(t, handler)
	require.NoError(t, handler(&pkgctx.CacheContext{
		Operation: pkgctx.CacheOperationInvalidate,
		OldID:     "OLD-001",
	}))
	validPath := filepath.Join(proj.Root, "valid_cache_obj.yaml")
	require.NoError(t, fileutil.WriteFile(validPath, []byte("test: true\n"), paths.FilePerm644))

	require.NoError(t, handler(&pkgctx.CacheContext{
		Operation: pkgctx.CacheOperationInvalidateAndUpdate,
		OldID:     "OLD-002",
		NewID:     "NEW-002",
		Kind:      "goal",
		FilePath:  validPath,
	}))
	require.NoError(t, handler(&pkgctx.CacheContext{
		Operation: "noop_op",
	}))

	blocker := filepath.Join(proj.Root, "blocker_file")
	require.NoError(t, fileutil.WriteFile(blocker, []byte("x"), paths.FilePerm644))
	_, err = createScenarioBuilder(cmd, blocker)
	require.Error(t, err)

	badTarget := filepath.Join(proj.Root, "bad_target")
	require.NoError(t, fileutil.MkdirAll(badTarget, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(badTarget, ".zqk"), []byte("x"), paths.FilePerm644))
	_, err = createScenarioBuilder(cmd, badTarget)
	require.Error(t, err)
}

func TestCycle141_FixHashes_CommandExecution_ErrorBranches(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Cleanup(func() {
		storage.SetCacheOperationHandler(nil)
		cli.UnregisterStorageForProjectRoot(proj.Root)
		storage.ShutdownHashRegistriesForProjectTesting(proj.Root)
	})

	// 1. Invalid flags (kind and internal together)
	cmd := NewFixHashesCmd()
	cmd.SetArgs([]string{"--kind", "goal", "--internal"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot use --kind and --internal together")

	// 2. Command with nil context
	cmd2 := NewFixHashesCmd()
	cli.SetContext(cmd2, nil)
	cmd2.SetArgs([]string{filepath.Join(proj.Root, "dummy.yaml")})
	err = cmd2.Execute()
	require.Error(t, err)

	// 3. FixHashes on non-existent file returns error on run
	cmd3 := NewFixHashesCmd()
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd3, cliCtx)
	cmd3.SetArgs([]string{filepath.Join(proj.Root, "nonexistent.yaml")})
	err = cmd3.Execute()
	require.Error(t, err)
}
