package utility

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"gopkg.in/yaml.v3"
)

func TestConfigFileWatcher_FullLifecycleAndEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	internalDir := filepath.Join(tmpDir, paths.ProcessInternalDir)
	require.NoError(t, fileutil.MkdirAll(internalDir, paths.DirPerm755))

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	watcher := NewConfigFileWatcher(tmpDir, coordinator, logger)
	require.NotNil(t, watcher)

	// Setup default watchers and verify files are registered
	watcher.SetupDefaultWatchers()
	assert.GreaterOrEqual(t, len(watcher.watchedFiles), 4)

	// Trigger reload callbacks to ensure they execute safely
	for _, state := range watcher.watchedFiles {
		if state.ReloadFn != nil {
			assert.NoError(t, state.ReloadFn())
		}
	}

	// Test RegisterFile with existing file
	sampleFile := filepath.Join(internalDir, "test_config.yaml")
	require.NoError(t, fileutil.WriteFile(sampleFile, []byte("key: value1\n"), paths.FilePerm644))

	reloaded := false
	watcher.RegisterFile("test_config.yaml", "config_test_changed", func() error {
		reloaded = true
		return nil
	})

	state, exists := watcher.watchedFiles["test_config.yaml"]
	require.True(t, exists)
	assert.False(t, state.MTime.IsZero())
	assert.Equal(t, int64(len("key: value1\n")), state.Size)

	// Test checkForChanges when file is modified
	ctx := pkgctx.NewSystemContext()
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, fileutil.WriteFile(sampleFile, []byte("key: value2_longer\n"), paths.FilePerm644))
	watcher.checkForChanges(ctx)
	assert.True(t, reloaded)

	// Test checkForChanges when file is deleted
	require.NoError(t, fileutil.Remove(sampleFile))
	watcher.checkForChanges(ctx)
	updatedState := watcher.watchedFiles["test_config.yaml"]
	assert.True(t, updatedState.MTime.IsZero())

	// Test checkForChanges when file is re-created
	require.NoError(t, fileutil.WriteFile(sampleFile, []byte("key: recreated\n"), paths.FilePerm644))
	watcher.checkForChanges(ctx)
	recreatedState := watcher.watchedFiles["test_config.yaml"]
	assert.False(t, recreatedState.MTime.IsZero())

	// Test start when already running
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	require.NoError(t, watcher.Start(runCtx))
	err := watcher.Start(runCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "watcher is already running")

	// Test stop idempotence
	watcher.Stop()
	watcher.Stop()

	// Test emitFileChangeEvent with nil coordinator
	nilCoordWatcher := NewConfigFileWatcher(tmpDir, nil, logger)
	nilCoordWatcher.emitFileChangeEvent(ctx, "file.yaml", fileState{Path: "/tmp/foo"}, nil, configWatcherChangeCreated)
}

func TestFixHashes_FlagsValidationAndReporting(t *testing.T) {

	// 1. validateFixHashesFlags
	err := validateFixHashesFlags(&FixHashesFlags{Kind: "backlog_item", Internal: true}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot use --kind and --internal together")

	err = validateFixHashesFlags(&FixHashesFlags{Kind: "backlog_item"}, []string{"path1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot specify paths when using --kind")

	err = validateFixHashesFlags(&FixHashesFlags{Internal: true}, []string{"path1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot specify paths when using --internal")

	err = validateFixHashesFlags(&FixHashesFlags{Kind: "backlog_item"}, nil)
	assert.NoError(t, err)

	// 2. showProgress
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(ctx)

	// Quiet suppresses output
	showProgress(cmd, true, 0, 50)
	assert.Empty(t, buf.String())

	// total > 10, current == 0
	showProgress(cmd, false, 0, 20)
	assert.Contains(t, buf.String(), "Processing 20 objects...")

	// total > 100, current % 100 == 0
	buf.Reset()
	showProgress(cmd, false, 100, 200)
	assert.Contains(t, buf.String(), "Processing 100/200 objects...")

	// 3. outputSummary
	buf.Reset()
	outputSummary(cmd, &FixHashesResult{
		FixedCount:   3,
		SkippedCount: 1,
		ErrorCount:   1,
		TotalFiles:   5,
		DryRun:       true,
		Errors:       []string{"sample_error.yaml: bad hash"},
		Quiet:        false,
	})
	out := buf.String()
	assert.Contains(t, out, "3 would be fixed")
	assert.Contains(t, out, "Dry run: no changes made.")
	assert.Contains(t, out, "sample_error.yaml: bad hash")

	// Normal success output
	buf.Reset()
	outputSummary(cmd, &FixHashesResult{
		FixedCount: 2,
		TotalFiles: 2,
		DryRun:     false,
		Quiet:      false,
	})
	assert.Contains(t, buf.String(), "All hashes fixed successfully")
}

func TestFixRegistration_ScanningAndEdgeCases(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})

	// 1. notifyScanning
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(ctx)

	notifyScanning(cmd, true)
	assert.Empty(t, buf.String())

	notifyScanning(cmd, false)
	assert.Contains(t, buf.String(), "Scanning objects for registration issues...")

	// 2. ensurePrefixForKind
	cfg := &validation.IDPrefixesConfig{
		KindToPrefixes: make(map[string][]string),
	}
	assert.True(t, ensurePrefixForKind(cfg, "backlog_item", "BLI"))
	assert.False(t, ensurePrefixForKind(cfg, "backlog_item", "BLI"))
	assert.True(t, ensurePrefixForKind(cfg, "backlog_item", "TASK"))

	// 3. applyRegistrationFix
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	issues := []registrationIssue{
		{
			ObjectID:       "CUSTOM-001",
			Kind:           "backlog_item",
			Message:        "ID format does not match kind backlog_item",
			Recommendation: "Update prefix config",
		},
	}

	// Missing config file
	_, err := applyRegistrationFix(cmd, proj.Root, issues, false, false, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "id_prefixes config not found")

	// Create valid id_prefixes config file
	configsDir := filepath.Join(proj.Root, paths.ProcessInternalConfigsDir)
	require.NoError(t, fileutil.MkdirAll(configsDir, paths.DirPerm755))
	configPath := filepath.Join(configsDir, paths.IdPrefixesConfigFile)
	initialConfig := validation.IDPrefixesConfig{
		KindToPrefixes: map[string][]string{
			"backlog_item": {"BLI"},
		},
	}
	data, err := yaml.Marshal(&initialConfig)
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(configPath, data, paths.FilePerm644))

	// Dry run does not mutate
	added, err := applyRegistrationFix(cmd, proj.Root, issues, true, false, logger)
	require.NoError(t, err)
	assert.Equal(t, []string{"backlog_item: CUSTOM-"}, added)

	// Real fix writes updated config
	added, err = applyRegistrationFix(cmd, proj.Root, issues, false, false, logger)
	require.NoError(t, err)
	assert.Equal(t, []string{"backlog_item: CUSTOM-"}, added)

	// Re-reading config confirms CUSTOM- is present
	updatedCfg, err := validation.LoadIDPrefixesConfig(configPath)
	require.NoError(t, err)
	assert.Contains(t, updatedCfg.KindToPrefixes["backlog_item"], "CUSTOM-")

	// 4. checkRegistrationIssue on non-existent object
	idVal := validation.GetIDValidator()
	require.NoError(t, idVal.LoadPatterns())
	issue := checkRegistrationIssue(proj.Root, "NONEXISTENT-999", idVal, logger)
	assert.Nil(t, issue)

	// 5. checkRegistrationIssue on valid object
	primaryDir := datacell.ProcessPrimaryDir(proj.Root)
	bliDir := filepath.Join(primaryDir, "backlog_items")
	require.NoError(t, fileutil.MkdirAll(bliDir, paths.DirPerm755))
	validBliFile := filepath.Join(bliDir, "BLI-101.yaml")
	require.NoError(t, fileutil.WriteFile(validBliFile, []byte("id: BLI-101\nkind: backlog_item\ntitle: Item\n"), paths.FilePerm644))

	issue = checkRegistrationIssue(proj.Root, "BLI-101", idVal, logger)
	assert.Nil(t, issue)

	// 6. findKindRegistrationIssues for kind with no directory
	kindIssues := findKindRegistrationIssues(proj.Root, "nonexistent_unknown_kind", idVal, logger)
	assert.Empty(t, kindIssues)
}

func TestValidateYAML_ElevationBranches(t *testing.T) {
	tmpDir := t.TempDir()

	cmd := &cobra.Command{}
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(ctx)

	// 1. processSingleYAMLFile with invalid YAML
	badFile := filepath.Join(tmpDir, "bad.yaml")
	require.NoError(t, fileutil.WriteFile(badFile, []byte("bad: [unterminated\n"), paths.FilePerm644))

	res := &ValidateYAMLResult{}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	cliCtx := cli.ContextForProjectRoot(tmpDir)
	processSingleYAMLFile(cmd, badFile, cliCtx, logger, &ValidateYAMLFlags{Quiet: false}, res)
	assert.Equal(t, 1, res.InvalidCount)
	assert.NotEmpty(t, res.Errors)

	// 2. processSingleYAMLFile with valid YAML, Quiet=true
	goodFile := filepath.Join(tmpDir, "good.yaml")
	require.NoError(t, fileutil.WriteFile(goodFile, []byte("title: Hello\n"), paths.FilePerm644))

	buf.Reset()
	res = &ValidateYAMLResult{}
	processSingleYAMLFile(cmd, goodFile, cliCtx, logger, &ValidateYAMLFlags{Quiet: true}, res)
	assert.Equal(t, 1, res.ValidCount)
	assert.Empty(t, buf.String())

	// 3. getRelativePath fallbacks
	assert.Equal(t, goodFile, getRelativePath("", goodFile))
}

func TestMigrate_ElevationBranches(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})

	cmd := NewMigrateCmd()
	initTestCmdContext(t, cmd, proj.Root, nil)

	// Test with source, target, phase flags set
	require.NoError(t, cmd.Flags().Set("source", proj.Root))
	require.NoError(t, cmd.Flags().Set("target", "file"))
	require.NoError(t, cmd.Flags().Set("phase", "all"))
	require.NoError(t, cmd.Flags().Set("version-constraint", ">=1.0.0"))

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(cmd.Context(), &buf)
	cmd.SetContext(ctx)

	// Fails with missing binary error in test environment
	err := runMigrate(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Migration tool not available")
}

func TestVersion_ElevationBranches(t *testing.T) {
	// Exercise SetVersionInfo and getExecutableName
	orig := versionInfo
	defer func() { versionInfo = orig }()

	SetVersionInfo("3.0.0-test", "2026-10-20T00:00:00Z", "abcdef999")
	assert.Equal(t, "3.0.0-test", versionInfo.Version)
	assert.NotEmpty(t, getExecutableName())
}

func TestScenarioBuilder_ElevationHelpers(t *testing.T) {

	// 1. validateCopyConfig
	assert.Error(t, validateCopyConfig(&ScenarioCopyConfig{}))
	assert.NoError(t, validateCopyConfig(&ScenarioCopyConfig{ObjectIDs: []string{"OBJ-1"}}))

	// 2. generateScenarioID
	assert.Equal(t, "SCN-MYSCENARIO", generateScenarioID("my-scenario"))

	// 3. getInitialStatusForScenario
	initStatus := getInitialStatusForScenario()
	assert.NotEmpty(t, initStatus)

	// 4. loadCountsFromConfig and loadDiversityFromConfig
	cfgMap := map[string]any{
		"counts": map[string]any{
			"goal": 5,
		},
		"diversity": map[string]any{
			"goal": 2,
		},
		"default_count":     42,
		"default_diversity": 7,
	}
	counts := loadCountsFromConfig(cfgMap)
	assert.Equal(t, 5, counts["goal"])

	diversity := loadDiversityFromConfig(cfgMap)
	assert.Equal(t, 2, diversity["goal"])

	assert.Equal(t, 42, loadDefaultCountFromConfig(cfgMap))
	assert.Equal(t, 7, loadDefaultDiversityFromConfig(cfgMap))
	assert.Equal(t, 0, loadDefaultCountFromConfig(map[string]any{}))
	assert.Equal(t, 0, loadDefaultDiversityFromConfig(map[string]any{}))

	// 5. matchesPIIPattern and shouldExcludeField
	assert.True(t, matchesPIIPattern("user_email", []string{"email", "phone"}))
	assert.False(t, matchesPIIPattern("title", []string{"email", "phone"}))

	alwaysEx := getAlwaysExcludeFields()
	assert.True(t, shouldExcludeField(objects.FieldKeyID, map[string]bool{objects.FieldKeyID: true}, nil, nil))
	assert.True(t, shouldExcludeField("secret", alwaysEx, map[string]bool{"secret": true}, nil))
	assert.True(t, shouldExcludeField("email_addr", alwaysEx, nil, []string{"email"}))
	assert.False(t, shouldExcludeField("description", alwaysEx, nil, []string{"email"}))

	// 6. applyFieldOverride and applyPreserveStateLogic
	copyCfg := &ScenarioCopyConfig{
		FieldOverrides: map[string]any{"status": "proposed"},
		PreserveState:  false,
	}
	assert.Equal(t, "proposed", applyFieldOverride("status", "in_progress", copyCfg))
	assert.Equal(t, "original", applyFieldOverride("other", "original", copyCfg))

	filtered := map[string]any{
		objects.FieldKeyCreatedAt: "2026-01-01",
		objects.FieldKeyCreatedBy: "alice",
	}
	applyPreserveStateLogic(filtered, copyCfg)
	assert.Empty(t, filtered[objects.FieldKeyCreatedAt])
	assert.Equal(t, scenarioBuilderAccountSystem, filtered[objects.FieldKeyCreatedBy])

	// 7. validateSourceDirectory
	tmpDir := t.TempDir()
	info, err := validateSourceDirectory(tmpDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	_, err = validateSourceDirectory(filepath.Join(tmpDir, "missing"))
	require.Error(t, err)

	filePath := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, fileutil.WriteFile(filePath, []byte("data"), paths.FilePerm644))
	_, err = validateSourceDirectory(filePath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")

	// 8. shouldSkipFile
	fileStat, err := fileutil.Stat(filePath)
	require.NoError(t, err)
	assert.False(t, shouldSkipFile(fileStat, "test.txt"))
	assert.True(t, shouldSkipFile(fileStat, ".hidden"))
	assert.True(t, shouldSkipFile(fileStat, ".hidden/nested.txt"))

	// 9. NewScenarioBuilder panics on nil coordinator
	assert.Panics(t, func() {
		NewScenarioBuilder(tmpDir, nil, nil)
	})
}
