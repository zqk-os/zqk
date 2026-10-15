package utility

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestNewScenarioBuilderCmd(t *testing.T) {
	cmd := NewScenarioBuilderCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "scenario-builder [flags]", cmd.Use)

	assert.NotNil(t, cmd.Flags().Lookup("target"))
	assert.NotNil(t, cmd.Flags().Lookup("scenario-id"))
	assert.NotNil(t, cmd.Flags().Lookup("data-file"))
	assert.NotNil(t, cmd.Flags().Lookup("kinds"))
	assert.NotNil(t, cmd.Flags().Lookup("count"))
	assert.NotNil(t, cmd.Flags().Lookup("default-count"))
	assert.NotNil(t, cmd.Flags().Lookup("diversity"))
	assert.NotNil(t, cmd.Flags().Lookup("default-diversity"))
	assert.NotNil(t, cmd.Flags().Lookup("link-probability"))
	assert.NotNil(t, cmd.Flags().Lookup("time-range"))
	assert.NotNil(t, cmd.Flags().Lookup("dry-run"))
	assert.NotNil(t, cmd.Flags().Lookup("force"))
	assert.NotNil(t, cmd.Flags().Lookup("object-ids"))
	assert.NotNil(t, cmd.Flags().Lookup("source-project"))
	assert.NotNil(t, cmd.Flags().Lookup("id-prefix-override"))
	assert.NotNil(t, cmd.Flags().Lookup("namespace-override"))
	assert.NotNil(t, cmd.Flags().Lookup("preserve-state"))
	assert.NotNil(t, cmd.Flags().Lookup("field-override"))
	assert.NotNil(t, cmd.Flags().Lookup("change-policy"))
}

func TestScenarioBuilder_FlagParsing(t *testing.T) {
	cmd := NewScenarioBuilderCmd()

	// Missing target flag
	_, err := parseScenarioBuilderFlags(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--target is required")

	// Target flag provided
	require.NoError(t, cmd.Flags().Set("target", "/tmp/test-target"))
	require.NoError(t, cmd.Flags().Set("kinds", "backlog_item,goal"))
	require.NoError(t, cmd.Flags().Set("count", "backlog_item=10,goal=5"))
	require.NoError(t, cmd.Flags().Set("diversity", "backlog_item=8,goal=3"))
	require.NoError(t, cmd.Flags().Set("field-override", "status=proposed"))

	flags, err := parseScenarioBuilderFlags(cmd)
	require.NoError(t, err)
	assert.Equal(t, "/tmp/test-target", flags.TargetDir)
	assert.Equal(t, []string{"backlog_item", "goal"}, flags.Kinds)
	assert.Equal(t, 10, flags.Counts["backlog_item"])
	assert.Equal(t, 5, flags.Counts["goal"])
	assert.Equal(t, 8, flags.Diversity["backlog_item"])
	assert.Equal(t, 3, flags.Diversity["goal"])
	assert.Equal(t, "proposed", flags.FieldOverrides["status"])

	// Test resolveTargetDirectory & validateTargetDirectory
	assert.Equal(t, "/tmp/test-target", resolveTargetDirectory("/tmp/test-target"))
	relResolved := resolveTargetDirectory("rel-path")
	assert.NotEmpty(t, relResolved)
	assert.NoError(t, validateTargetDirectory(relResolved))

	// Invalid count format
	invalidCountCmd := NewScenarioBuilderCmd()
	require.NoError(t, invalidCountCmd.Flags().Set("target", "/tmp/target"))
	require.NoError(t, invalidCountCmd.Flags().Set("count", "backlog_item=notanumber"))
	_, err = parseScenarioBuilderFlags(invalidCountCmd)
	assert.Error(t, err)

	// Invalid diversity format
	invalidDivCmd := NewScenarioBuilderCmd()
	require.NoError(t, invalidDivCmd.Flags().Set("target", "/tmp/target"))
	require.NoError(t, invalidDivCmd.Flags().Set("diversity", "backlog_item=notanumber"))
	_, err = parseScenarioBuilderFlags(invalidDivCmd)
	assert.Error(t, err)
}

func TestScenarioBuilder_ConfigMethods(t *testing.T) {
	tmpDir := t.TempDir()
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	storageProvider := setupTestStorageProvider(t, tmpDir)
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)

	now := time.Now()
	builder.
		WithKinds("goal", "milestone").
		WithCount("goal", 25).
		WithDefaultCount(50).
		WithDiversity("goal", 15).      // Should clamp to 10
		WithDiversity("milestone", -2). // Should clamp to 1
		WithDefaultDiversity(12).       // Should clamp to 10
		WithLinkProbability(1.5).       // Should clamp to 1.0
		WithTimeRange(48 * time.Hour).
		WithStartTime(now)

	assert.Equal(t, []string{"goal", "milestone"}, builder.config.Kinds)
	assert.Equal(t, 25, builder.config.Counts["goal"])
	assert.Equal(t, 50, builder.config.DefaultCount)
	assert.Equal(t, 10, builder.config.Diversity["goal"])
	assert.Equal(t, 1, builder.config.Diversity["milestone"])
	assert.Equal(t, 10, builder.config.DefaultDiversity)
	assert.Equal(t, 1.0, builder.config.LinkProbability)
	assert.Equal(t, 48*time.Hour, builder.config.TimeRange)
	assert.Equal(t, now, builder.config.StartTime)

	builder.WithDefaultDiversity(0) // Should clamp to 1
	assert.Equal(t, 1, builder.config.DefaultDiversity)

	builder.WithLinkProbability(-0.5) // Should clamp to 0.0
	assert.Equal(t, 0.0, builder.config.LinkProbability)
}

func TestRunScenarioBuilder_ExecutionFlows(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	targetDir := filepath.Join(proj.Root, "test-target")

	// 1. Dry run with kinds
	cmd := NewScenarioBuilderCmd()
	initTestCmdContext(t, cmd, proj.Root, nil)
	require.NoError(t, cmd.Flags().Set("target", targetDir))
	require.NoError(t, cmd.Flags().Set("kinds", "backlog_item,goal"))
	require.NoError(t, cmd.Flags().Set("dry-run", "true"))

	err := runScenarioBuilder(cmd, []string{})
	require.NoError(t, err)

	// 2. Missing kinds when not using scenario-id, data-file or object-ids
	noKindsCmd := NewScenarioBuilderCmd()
	initTestCmdContext(t, noKindsCmd, proj.Root, nil)
	require.NoError(t, noKindsCmd.Flags().Set("target", targetDir))
	err = runScenarioBuilder(noKindsCmd, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--kinds is required")

	// 3. Dry run with object-ids
	objCmd := NewScenarioBuilderCmd()
	initTestCmdContext(t, objCmd, proj.Root, nil)
	require.NoError(t, objCmd.Flags().Set("target", targetDir))
	require.NoError(t, objCmd.Flags().Set("object-ids", "BLI-001,GOAL-001"))
	require.NoError(t, objCmd.Flags().Set("source-project", proj.Root))
	require.NoError(t, objCmd.Flags().Set("dry-run", "true"))

	err = runScenarioBuilder(objCmd, []string{})
	require.NoError(t, err)

	// 4. Data-file loading error on empty file
	emptyDataFile := filepath.Join(proj.Root, "empty_data.yaml")
	require.NoError(t, fileutil.WriteFile(emptyDataFile, []byte(""), paths.FilePerm644))

	dataCmd := NewScenarioBuilderCmd()
	initTestCmdContext(t, dataCmd, proj.Root, nil)
	require.NoError(t, dataCmd.Flags().Set("target", targetDir))
	require.NoError(t, dataCmd.Flags().Set("data-file", emptyDataFile))

	err = runScenarioBuilder(dataCmd, []string{})
	assert.Error(t, err)

	// 5. Data-file loading success with valid objects array
	validDataFile := filepath.Join(proj.Root, "valid_data.yaml")
	validYAML := `- kind: backlog_item
  id: BLI-001
  title: Test BLI
  status: planned
`
	require.NoError(t, fileutil.WriteFile(validDataFile, []byte(validYAML), paths.FilePerm644))

	validDataCmd := NewScenarioBuilderCmd()
	initTestCmdContext(t, validDataCmd, proj.Root, nil)
	require.NoError(t, validDataCmd.Flags().Set("target", targetDir))
	require.NoError(t, validDataCmd.Flags().Set("data-file", validDataFile))

	err = runScenarioBuilder(validDataCmd, []string{})
	require.NoError(t, err)
}

func TestCreateScenarioBuilder_Direct(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	cmd := NewScenarioBuilderCmd()
	initTestCmdContext(t, cmd, proj.Root, nil)
	targetDir := filepath.Join(proj.Root, "test-target-builder")

	builder, err := createScenarioBuilder(cmd, targetDir)
	require.NoError(t, err)
	assert.NotNil(t, builder)
	assert.Equal(t, targetDir, builder.config.TargetDir)
}
