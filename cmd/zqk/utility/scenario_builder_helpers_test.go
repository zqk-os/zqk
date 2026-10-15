package utility

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestPIIHelpers(t *testing.T) {
	// identifyPIIFieldsFromSpec with nil and empty
	assert.Empty(t, identifyPIIFieldsFromSpec(nil))
	assert.Empty(t, identifyPIIFieldsFromSpec(&objects.Spec{}))

	spec := &objects.Spec{
		Fields: map[string]any{
			"email_address": map[string]any{
				"checklist": map[string]any{
					"security": "confidential_pii",
				},
			},
			"normal_field": map[string]any{
				"checklist": map[string]any{
					"security": "public",
				},
			},
		},
	}
	pii := identifyPIIFieldsFromSpec(spec)
	assert.True(t, pii["email_address"])
	assert.False(t, pii["normal_field"])

	// patterns
	patterns := getCommonPIIPatterns()
	assert.NotEmpty(t, patterns)
	assert.True(t, matchesPIIPattern("user_email", patterns))
	assert.True(t, matchesPIIPattern("api_key", patterns))
	assert.False(t, matchesPIIPattern("status", patterns))

	// shouldExcludeField
	always := getAlwaysExcludeFields()
	assert.False(t, shouldExcludeField("status", always, pii, patterns))
	assert.True(t, shouldExcludeField("user_email", always, pii, patterns))
	assert.True(t, shouldExcludeField("email_address", always, pii, patterns))

	// applyFieldOverride
	cfg := &ScenarioCopyConfig{
		FieldOverrides: map[string]any{
			"status": "draft",
		},
	}
	assert.Equal(t, "draft", applyFieldOverride("status", "active", cfg))
	assert.Equal(t, "other", applyFieldOverride("name", "other", cfg))

	// applyPreserveStateLogic
	m := map[string]any{"status": "active", "created_at": "old"}
	cfg.PreserveState = false
	applyPreserveStateLogic(m, cfg)
	assert.Equal(t, "", m["created_at"])

	m2 := map[string]any{"status": "active", "created_at": "old"}
	cfg.PreserveState = true
	applyPreserveStateLogic(m2, cfg)
	assert.Equal(t, "old", m2["created_at"])
}

func TestLoadHelpers(t *testing.T) {
	// convertToInt
	i, ok := convertToInt(42)
	assert.True(t, ok)
	assert.Equal(t, 42, i)

	f, ok := convertToInt(float64(100))
	assert.True(t, ok)
	assert.Equal(t, 100, f)

	_, ok = convertToInt("not-int")
	assert.False(t, ok)

	// load functions from config map
	configMap := map[string]any{
		"kinds":             []any{"backlog_item", "goal"},
		"counts":            map[string]any{"backlog_item": 20, "goal": float64(5)},
		"diversity":         map[string]any{"backlog_item": 7, "goal": float64(4)},
		"default_count":     float64(50),
		"default_diversity": float64(6),
		"link_probability":  0.45,
		"time_range":        "48h",
		"start_time":        "2026-01-01T00:00:00Z",
	}

	kinds := loadKindsFromConfig(configMap)
	assert.Equal(t, []string{"backlog_item", "goal"}, kinds)

	counts := loadCountsFromConfig(configMap)
	assert.Equal(t, 20, counts["backlog_item"])
	assert.Equal(t, 5, counts["goal"])

	divs := loadDiversityFromConfig(configMap)
	assert.Equal(t, 7, divs["backlog_item"])
	assert.Equal(t, 4, divs["goal"])

	assert.Equal(t, 50, loadDefaultCountFromConfig(configMap))
	assert.Equal(t, 6, loadDefaultDiversityFromConfig(configMap))
	assert.Equal(t, 0.45, loadLinkProbabilityFromConfig(configMap))
	assert.Equal(t, 48*time.Hour, loadTimeRangeFromConfig(configMap))

	st := loadStartTimeFromConfig(configMap)
	assert.False(t, st.IsZero())

	// applyConfigToBuilder
	tmpDir := t.TempDir()
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	storageProvider := setupTestStorageProvider(t, tmpDir)
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)

	applyConfigToBuilder(builder, configMap)
	assert.Equal(t, []string{"backlog_item", "goal"}, builder.config.Kinds)
	assert.Equal(t, 20, builder.config.Counts["backlog_item"])
	assert.Equal(t, 7, builder.config.Diversity["backlog_item"])
	assert.Equal(t, 50, builder.config.DefaultCount)
	assert.Equal(t, 6, builder.config.DefaultDiversity)
	assert.Equal(t, 0.45, builder.config.LinkProbability)
	assert.Equal(t, 48*time.Hour, builder.config.TimeRange)

	// extractDataGenerationConfig
	extracted, err := extractDataGenerationConfig(map[string]any{
		objects.FieldKeyDataGenerationConfig: configMap,
	}, "SCN-001")
	require.NoError(t, err)
	assert.NotNil(t, extracted)

	// Missing data_generation_config
	_, err = extractDataGenerationConfig(map[string]any{}, "SCN-001")
	assert.Error(t, err)

	// Invalid type
	_, err = extractDataGenerationConfig(map[string]any{
		objects.FieldKeyDataGenerationConfig: "not-a-map",
	}, "SCN-001")
	assert.Error(t, err)
}

func TestScenarioBuilder_ScenarioManagement(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	tmpDir := proj.Root

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	storageProvider := setupTestStorageProvider(t, tmpDir)
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	// Test helper functions
	assert.Equal(t, "SCN-MYTEST", generateScenarioID("my-test"))
	assert.NotEmpty(t, getInitialStatusForScenario())

	obj := builder.buildNewScenarioObject("test-run", "SCE-001", "active")
	assert.Equal(t, "SCE-001", obj[objects.FieldKeyID])
	assert.Equal(t, objects.KindScenario, obj[objects.FieldKeyKind])

	// createNewScenario with valid ID SCE-001
	ctx := storage.WithSkipWriteBehind(context.Background())
	err := builder.createNewScenario(ctx, "alpha-scenario", "SCE-001")
	require.NoError(t, err)

	// Read it back and verify
	readObj, id, exists := builder.findExistingScenarioByIDOrName(ctx, "SCE-001", "alpha-scenario")
	assert.True(t, exists)
	assert.Equal(t, "SCE-001", id)
	assert.NotNil(t, readObj)

	// Update existing scenario
	err = builder.updateExistingScenario(ctx, readObj)
	require.NoError(t, err)

	// createOrUpdateScenarioObject (should update existing)
	err = builder.createOrUpdateScenarioObject(ctx, "alpha-scenario", "SCE-001")
	require.NoError(t, err)

	// LoadFromScenarioObject
	err = builder.LoadFromScenarioObject(ctx, "SCE-001")
	require.NoError(t, err)

	// findScenarioByName with ID
	foundID, err := builder.findScenarioByName(ctx, "SCE-001")
	require.NoError(t, err)
	assert.Equal(t, "SCE-001", foundID)

	// findScenarioByName - not found
	_, err = builder.findScenarioByName(ctx, "nonexistent-scenario")
	assert.Error(t, err)
}

func TestLifecycleCreator(t *testing.T) {
	tmpDir := t.TempDir()
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	storageProvider := setupTestStorageProvider(t, tmpDir)
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.config.TargetDir = tmpDir
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	// 1. Non-existent directory -> should return nil gracefully
	ctx := context.Background()
	err := builder.createLifecycleObjects(ctx)
	assert.NoError(t, err)

	// 2. Test abbreviation helper
	assert.Equal(t, "BLI", getObjectTypeAbbreviationForLifecycle("backlog_item"))
	assert.Equal(t, "CUSTOM", getObjectTypeAbbreviationForLifecycle("custom"))

	// 3. Create lifecycles directory with valid lifecycle file
	lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
	require.NoError(t, fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755))

	lifecycleContent := `object_type: task
statuses:
  - value: draft
    origin: true
  - value: in_progress
  - value: complete
    terminal: true
`
	lifecycleFile := filepath.Join(lifecyclesDir, "task_lifecycle.yaml")
	require.NoError(t, fileutil.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644))

	// Also add a .bak file to test skipping backup files
	bakFile := filepath.Join(lifecyclesDir, "task_lifecycle.yaml.bak")
	require.NoError(t, fileutil.WriteFile(bakFile, []byte("backup"), paths.FilePerm644))

	err = builder.createLifecycleObjects(ctx)
	assert.NoError(t, err)
}
