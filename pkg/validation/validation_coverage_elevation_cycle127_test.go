package validation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func setupIsolatedTestEnv(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), tempDir)
	t.Setenv(zqkenv.TestRoot().Name(), tempDir)
	return tempDir
}

func TestCycle127_StateCache_CountAutoFixableAndLoadedUpdated(t *testing.T) {
	setupIsolatedTestEnv(t)
	cache := &ValidationStateCache{
		cache: make(map[string]*ValidationState),
	}

	assert.Equal(t, 0, cache.CountAutoFixable())

	now := time.Now()
	cache.loadedUpdated = now
	assert.Equal(t, now, cache.LoadedUpdated())

	// State without issues
	cache.cache["obj-1"] = &ValidationState{
		ObjectID: "obj-1",
		Issues:   []ValidationIssue{},
	}
	assert.Equal(t, 0, cache.CountAutoFixable())

	// State with resolved autofixable issue
	resolvedTime := time.Now()
	cache.cache["obj-2"] = &ValidationState{
		ObjectID: "obj-2",
		Issues: []ValidationIssue{
			{AutoFixable: true, ResolvedAt: &resolvedTime},
		},
	}
	assert.Equal(t, 0, cache.CountAutoFixable())

	// State with unresolved non-autofixable issue
	cache.cache["obj-3"] = &ValidationState{
		ObjectID: "obj-3",
		Issues: []ValidationIssue{
			{AutoFixable: false, ResolvedAt: nil},
		},
	}
	assert.Equal(t, 0, cache.CountAutoFixable())

	// State with unresolved autofixable issue
	cache.cache["obj-4"] = &ValidationState{
		ObjectID: "obj-4",
		Issues: []ValidationIssue{
			{AutoFixable: true, ResolvedAt: nil},
		},
	}
	assert.Equal(t, 1, cache.CountAutoFixable())
}

func TestCycle127_StateCache_ShrinkGuardSkipSave(t *testing.T) {
	tempDir := setupIsolatedTestEnv(t)
	cacheFile := filepath.Join(tempDir, "validation_cache.json")

	// Case 1: newCount == 0 returns false
	byKindEmpty := map[string]kindBucket{}
	assert.False(t, shrinkGuardSkipSave(cacheFile, byKindEmpty))

	// Case 2: newCount >= 200 returns false
	entries200 := make([]validationStateEntry, 201)
	byKindLarge := map[string]kindBucket{"goal": {Entries: entries200}}
	assert.False(t, shrinkGuardSkipSave(cacheFile, byKindLarge))

	// Case 3: File does not exist returns false
	byKindSmall := map[string]kindBucket{"goal": {Entries: make([]validationStateEntry, 10)}}
	assert.False(t, shrinkGuardSkipSave(cacheFile, byKindSmall))

	// Case 4: File invalid JSON returns false
	err := fileutil.WriteFile(cacheFile, []byte("invalid-json"), 0644)
	require.NoError(t, err)
	assert.False(t, shrinkGuardSkipSave(cacheFile, byKindSmall))

	// Case 5: File exists with existingCount < shrinkGuardMinDiskEntries (e.g. 50) returns false
	diskCacheSmall := map[string]any{
		"by_kind": map[string]any{
			"goal": map[string]any{
				"entries": make([]map[string]any, 50),
			},
		},
	}
	data, err := json.Marshal(diskCacheSmall)
	require.NoError(t, err)
	err = fileutil.WriteFile(cacheFile, data, 0644)
	require.NoError(t, err)
	assert.False(t, shrinkGuardSkipSave(cacheFile, byKindSmall))

	// Case 6: File exists with 300 entries and newCount=10 (< 200 and 10*2 < 300) returns true
	diskCacheLarge := map[string]any{
		"by_kind": map[string]any{
			"goal": map[string]any{
				"entries": make([]map[string]any, 300),
			},
		},
	}
	dataLarge, err := json.Marshal(diskCacheLarge)
	require.NoError(t, err)
	err = fileutil.WriteFile(cacheFile, dataLarge, 0644)
	require.NoError(t, err)
	assert.True(t, shrinkGuardSkipSave(cacheFile, byKindSmall))
}

func TestCycle127_StateCache_StaleGenerationSkipSave(t *testing.T) {
	tempDir := setupIsolatedTestEnv(t)
	cacheFile := filepath.Join(tempDir, "validation_cache.json")

	// Case 1: nil cache returns false
	assert.False(t, staleGenerationSkipSave(nil))

	// Case 2: replaceDiskOnNextSave == true returns false
	cache := &ValidationStateCache{
		cacheFile:             cacheFile,
		replaceDiskOnNextSave: true,
		loadedUpdated:         time.Now().Add(-1 * time.Hour),
	}
	assert.False(t, staleGenerationSkipSave(cache))

	// Case 3: Cache file does not exist returns false
	cache.replaceDiskOnNextSave = false
	assert.False(t, staleGenerationSkipSave(cache))

	// Case 4: Cache file contains invalid JSON returns false
	err := fileutil.WriteFile(cacheFile, []byte("bad-json"), 0644)
	require.NoError(t, err)
	assert.False(t, staleGenerationSkipSave(cache))

	// Case 5: Cache file exists with older or equal updated returns false
	loadedTime := time.Now()
	cache.loadedUpdated = loadedTime
	olderPayload := fmt.Sprintf(`{"updated":"%s"}`, loadedTime.Add(-10*time.Minute).Format(time.RFC3339))
	err = fileutil.WriteFile(cacheFile, []byte(olderPayload), 0644)
	require.NoError(t, err)
	assert.False(t, staleGenerationSkipSave(cache))

	// Case 6: Cache file exists with newer updated returns true
	newerPayload := fmt.Sprintf(`{"updated":"%s"}`, loadedTime.Add(10*time.Minute).Format(time.RFC3339))
	err = fileutil.WriteFile(cacheFile, []byte(newerPayload), 0644)
	require.NoError(t, err)
	assert.True(t, staleGenerationSkipSave(cache))
}

func TestCycle127_StateCache_CommonPathPrefix(t *testing.T) {
	// Empty slice
	assert.Equal(t, "", commonPathPrefix(nil))
	assert.Equal(t, "", commonPathPrefix([]string{}))

	// Single path without separator
	assert.Equal(t, "", commonPathPrefix([]string{"filename.txt"}))

	// Single path with directory
	assert.Equal(t, "a/b/", commonPathPrefix([]string{"a/b/c.txt"}))

	// Multiple paths with matching directory
	paths := []string{"pkg/validation/a.go", "pkg/validation/b.go", "pkg/validation/sub/c.go"}
	assert.Equal(t, "pkg/validation/", commonPathPrefix(paths))

	// Multiple paths with no common directory
	pathsDiff := []string{"pkg/validation/a.go", "cmd/zqk/main.go"}
	assert.Equal(t, "", commonPathPrefix(pathsDiff))

	// Path with shorter length
	pathsShort := []string{"pkg/validation/sub/a.go", "pkg/"}
	assert.Equal(t, "pkg/", commonPathPrefix(pathsShort))
}

func TestCycle127_StateCache_SharedCacheOps(t *testing.T) {
	tempDir := setupIsolatedTestEnv(t)
	ctxDefault := pkgctx.NewSystemContext()

	// GetValidationStateForTest without TestSync mode returns false
	state, ok := GetValidationStateForTest(ctxDefault, tempDir, "GOL-001")
	assert.False(t, ok)
	assert.Nil(t, state)

	// Context with TestSync mode
	ctxSync := pkgctx.WithCacheMode(ctxDefault, pkgctx.CacheModeTestSync)

	// UpdateObject with empty project root / nil state / invalid IDs
	UpdateObjectInGlobalValidationCacheWithContext(ctxSync, "", nil)
	UpdateObjectInGlobalValidationCacheWithContext(ctxSync, tempDir, nil)
	UpdateObjectInGlobalValidationCacheWithContext(ctxSync, tempDir, &ValidationState{ObjectID: ""})

	longID := "GOL-" + string(make([]byte, MaxObjectIDLength+1))
	UpdateObjectInGlobalValidationCacheWithContext(ctxSync, tempDir, &ValidationState{ObjectID: longID})

	// Non-cacheable kind
	UpdateObjectInGlobalValidationCacheWithContext(ctxSync, tempDir, &ValidationState{
		ObjectID:   "LOG-001",
		ObjectKind: "audit_log",
	})

	// Valid cacheable object update
	validState := &ValidationState{
		ObjectID:      "GOL-127-TEST",
		ObjectKind:    objects.KindGoal,
		FilePath:      filepath.Join(tempDir, "goals", "goal.yaml"),
		LastValidated: time.Now(),
	}
	UpdateObjectInGlobalValidationCacheWithContext(ctxSync, tempDir, validState)

	gotState, ok := GetValidationStateForTest(ctxSync, tempDir, "GOL-127-TEST")
	assert.True(t, ok)
	require.NotNil(t, gotState)
	assert.Equal(t, "GOL-127-TEST", gotState.ObjectID)
}

func TestCycle127_GoValidator_CheckAtLeastSinglePrecondition(t *testing.T) {
	setupIsolatedTestEnv(t)
	gv := NewGoValidator()

	obj := map[string]any{
		"items": []any{"a", "b"},
		"title": "A Title",
	}

	// At least 1 items -> true
	met := gv.checkAtLeastSinglePrecondition("at_least:1:items", obj)
	assert.True(t, met)

	// At least 3 items -> false
	met = gv.checkAtLeastSinglePrecondition("at_least:3:items", obj)
	assert.False(t, met)
}

func TestCycle127_GoValidator_StringRefsList(t *testing.T) {
	assert.Nil(t, stringRefsList(nil))
	assert.Nil(t, stringRefsList(""))
	assert.Equal(t, []string{"ABC-1"}, stringRefsList("ABC-1"))
	assert.Equal(t, []string{"A", "B"}, stringRefsList([]string{"A", "B"}))

	anySlice := []any{"A", 123, "B", nil}
	assert.Equal(t, []string{"A", "B"}, stringRefsList(anySlice))

	assert.Nil(t, stringRefsList(42))
}

func TestCycle127_GoValidator_DetectCycle(t *testing.T) {
	setupIsolatedTestEnv(t)
	gv := NewGoValidator()

	// Direct cycle: current == target
	found, hops := gv.detectCycle("A", "A", map[string]bool{}, map[string]bool{}, nil)
	assert.True(t, found)
	assert.Equal(t, 0, hops)

	// visiting or fullyExplored returns false
	found, _ = gv.detectCycle("A", "B", map[string]bool{"A": true}, map[string]bool{}, nil)
	assert.False(t, found)
	found, _ = gv.detectCycle("A", "B", map[string]bool{}, map[string]bool{"A": true}, nil)
	assert.False(t, found)

	// Object lookup error returns false
	optionsErr := &ValidationOptions{
		ObjectLookup: func(id string) (map[string]any, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	found, _ = gv.detectCycle("A", "B", map[string]bool{}, map[string]bool{}, optionsErr)
	assert.False(t, found)

	// Multi-hop cycle: A -> B -> target C
	store := map[string]map[string]any{
		"node-a": {"next_ref": "node-b"},
		"node-b": {"target_ref": "node-c"},
		"node-c": {"self_ref": "node-c"},
	}
	opts := &ValidationOptions{
		ObjectLookup: func(id string) (map[string]any, error) {
			if obj, ok := store[id]; ok {
				return obj, nil
			}
			return nil, fmt.Errorf("id %s not found", id)
		},
	}

	found, hops = gv.detectCycle("node-a", "node-c", map[string]bool{}, map[string]bool{}, opts)
	assert.True(t, found)
	assert.Equal(t, 2, hops)
}

func TestCycle127_GoValidator_MachineCheckableClosureEvidence(t *testing.T) {
	setupIsolatedTestEnv(t)
	gv := NewGoValidator()

	// Missing root returns false
	assert.False(t, gv.checkMachineCheckableClosureEvidence(map[string]any{}, nil))

	// Grandfathered: current state complete and target status complete returns true
	objComplete := map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusComplete,
	}
	optsGrandfathered := &ValidationOptions{
		CurrentState: objects.ObjectStatusComplete,
	}
	assert.True(t, gv.checkMachineCheckableClosureEvidence(objComplete, optsGrandfathered))

	// Non-complete transition without root returns false
	optsInProgress := &ValidationOptions{
		CurrentState: objects.ObjectStatusInProgress,
	}
	assert.False(t, gv.checkMachineCheckableClosureEvidence(objComplete, optsInProgress))

	// stringSliceField helper
	assert.Nil(t, stringSliceField(nil))
	assert.Equal(t, []string{"a", "b"}, stringSliceField([]string{"a", "b"}))
	assert.Equal(t, []string{"x", "y"}, stringSliceField([]any{"x", "y", "", 123}))
}

func TestCycle127_GoValidator_EvalActiveRefStage(t *testing.T) {
	setupIsolatedTestEnv(t)
	gv := NewGoValidator()

	// Precondition not containing SubprecondActive returns false, false
	h, m := evalActiveRefStage(gv, "status:complete", map[string]any{}, nil)
	assert.False(t, h)
	assert.False(t, m)

	// Precondition with active_ref but no ref fields returns false, false
	h, m = evalActiveRefStage(gv, "active_ref:something", map[string]any{}, nil)
	assert.False(t, h)
	assert.False(t, m)

	// With goal_ref and options checking active
	obj := map[string]any{
		"goal_ref": "GOL-001",
	}
	opts := &ValidationOptions{
		ObjectStatusLookup: func(id string) (string, error) {
			return objects.ObjectStatusInProgress, nil
		},
	}
	h, m = evalActiveRefStage(gv, "active goal_ref", obj, opts)
	assert.True(t, h)
	assert.True(t, m)

	// With slice of string refs
	objSlice := map[string]any{
		"goal_refs": []string{"GOL-001", "GOL-002"},
	}
	h, m = evalActiveRefStage(gv, "active goal_ref", objSlice, opts)
	assert.True(t, h)
	assert.True(t, m)

	// With slice of any refs
	objAnySlice := map[string]any{
		"goal_refs": []any{"GOL-001", "GOL-002"},
	}
	h, m = evalActiveRefStage(gv, "active goal_ref", objAnySlice, opts)
	assert.True(t, h)
	assert.True(t, m)
}

func TestCycle127_OwnerRefPrototype_ContainsString(t *testing.T) {
	assert.True(t, containsString("alpha-beta-gamma", "beta"))
	assert.True(t, containsString("alpha-beta-gamma", "xyz", "gamma"))
	assert.False(t, containsString("alpha-beta-gamma", "delta", "omega"))
	assert.False(t, containsString("alpha-beta-gamma"))

	// Validate prototype account ref
	assert.True(t, validatePrototypeAccountRef(""))
	assert.True(t, validatePrototypeAccountRef("prod-team-lead"))
	assert.False(t, validatePrototypeAccountRef("test-account"))
	assert.False(t, validatePrototypeAccountRef("mock"))
}

func TestCycle127_OverlayDSL_ConstraintsAndStages(t *testing.T) {
	setupIsolatedTestEnv(t)
	gv := NewGoValidator()

	// evalAtLeastConstraint invalid arg (no colon)
	h, m := evalAtLeastConstraint(map[string]any{}, "invalid_arg", gv)
	assert.True(t, h)
	assert.False(t, m)

	// evalAtLeastConstraint gv == nil fallback
	obj := map[string]any{
		"items": []any{"x", "y"},
		"name":  "single_value",
	}
	h, m = evalAtLeastConstraint(obj, "2:items", nil)
	assert.True(t, h)
	assert.True(t, m)

	h, m = evalAtLeastConstraint(obj, "1:name", nil)
	assert.True(t, h)
	assert.True(t, m)

	h, m = evalAtLeastConstraint(obj, "1:missing", nil)
	assert.True(t, h)
	assert.False(t, m)

	// evalPlanRefStatusStage
	assertHelperPlanRefStatus(t, gv)

	// evalCodeEvidenceStage
	assertHelperCodeEvidence(t, gv)
}

func assertHelperPlanRefStatus(t *testing.T, gv *GoValidator) {
	t.Helper()
	h, m := evalPlanRefStatusStage(nil, map[string]any{}, nil, "priority_plan_execution_facing")
	assert.False(t, h)
	assert.False(t, m)

	h, _ = evalPlanRefStatusStage(gv, map[string]any{}, nil, "linked_criteria_validated_or_complete")
	assert.True(t, h)

	h, _ = evalPlanRefStatusStage(gv, map[string]any{}, nil, "priority_plan_archived_when_set")
	assert.True(t, h)

	h, _ = evalPlanRefStatusStage(gv, map[string]any{}, nil, "priority_plan_execution_facing")
	assert.True(t, h)

	h, m = evalPlanRefStatusStage(gv, map[string]any{}, nil, "unknown_rule")
	assert.False(t, h)
	assert.False(t, m)
}

func assertHelperCodeEvidence(t *testing.T, gv *GoValidator) {
	t.Helper()
	h, _ := evalCodeEvidenceStage(gv, map[string]any{}, nil, "git_mutation_evidence_present")
	assert.True(t, h)

	h, _ = evalCodeEvidenceStage(gv, map[string]any{}, nil, "branch_is_ancestor_of_trunk")
	assert.True(t, h)

	h, _ = evalCodeEvidenceStage(gv, map[string]any{}, nil, "machine_checkable_closure_evidence")
	assert.True(t, h)

	h, m := evalCodeEvidenceStage(gv, map[string]any{"work_done": true}, nil, "work_done")
	assert.True(t, h)
	assert.True(t, m)

	h, m = evalCodeEvidenceStage(gv, map[string]any{"work_done": false, "status": "draft"}, nil, "work_done")
	assert.True(t, h)
	assert.False(t, m)

	h, m = evalCodeEvidenceStage(gv, map[string]any{}, nil, "unknown_code_evidence")
	assert.False(t, h)
	assert.False(t, m)
}

func TestCycle127_OverlayDSL_EvalRoleIsAndWorkflowStage(t *testing.T) {
	setupIsolatedTestEnv(t)
	gv := NewGoValidator()

	// evalRoleIs with status and kind
	obj := map[string]any{
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyStatus: objects.ObjectStatusDraft,
	}
	h, _ := evalRoleIs(gv, obj, "draft")
	assert.True(t, h)

	// evalOverlayWorkflowStage
	h, _ = evalOverlayWorkflowStage(gv, obj, nil, "role_is", "draft")
	assert.True(t, h)

	h, _ = evalOverlayWorkflowStage(gv, obj, nil, "shovel_ready", "")
	assert.True(t, h)

	h, _ = evalOverlayWorkflowStage(gv, obj, nil, "tdd_test_red_phase", "")
	assert.True(t, h)

	h, _ = evalOverlayWorkflowStage(gv, obj, nil, "criteria_active_test_case", "")
	assert.True(t, h)

	h, _ = evalOverlayWorkflowStage(gv, obj, nil, "ready_backlog_references_plan", "")
	assert.True(t, h)

	h, _ = evalOverlayWorkflowStage(gv, obj, nil, "workflow_constraints_if_set", "")
	assert.True(t, h)

	h, _ = evalOverlayWorkflowStage(gv, obj, nil, "priority_plan_validated", "")
	assert.True(t, h)
}

func TestCycle127_AsyncValidator_MetricsAndShutdown(t *testing.T) {
	tempDir := setupIsolatedTestEnv(t)
	metrics := noopAsyncValidatorShutdownMetrics{}
	metrics.RecordStage(context.Background(), "test-pipeline", "stage-1", time.Millisecond, nil)
	metrics.RecordStageWithBuckets(context.Background(), "test-pipeline", "stage-1", time.Millisecond, nil, map[string]string{"env": "test"})

	// ValidationPool ConcurrencyLimit
	var nilPool *ValidationPool
	assert.Equal(t, 1, nilPool.ConcurrencyLimit())

	pool := NewValidationPool(4)
	assert.Equal(t, 4, pool.ConcurrencyLimit())

	// AsyncValidator shutdown methods
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	av := NewAsyncValidator(ctx, tempDir, 2, time.Minute)
	defer av.Stop()
	assert.Equal(t, "async_validator", av.GetName())
	assert.True(t, av.IsCritical())
	assert.Equal(t, int64(0), av.GetPendingCount())
	assert.True(t, av.IsDrained())

	err := av.Drain(context.Background())
	require.NoError(t, err)
}

func TestCycle127_AsyncValidatorWorker_ReadValidationInput(t *testing.T) {
	tempDir := setupIsolatedTestEnv(t)
	filePath := filepath.Join(tempDir, "sample.json")
	payload := `{"test_id":"TEST-123","status":"active"}`
	err := fileutil.WriteFile(filePath, []byte(payload), 0644)
	require.NoError(t, err)

	// Standard path
	data, err := readValidationInput(filePath)
	require.NoError(t, err)
	assert.Equal(t, payload, string(data))

	// Segment path with bad offset (falls back to reading file with ::not_a_number, which doesn't exist)
	badSegmentPath := filePath + "::not_a_number"
	_, err = readValidationInput(badSegmentPath)
	assert.Error(t, err)

	// Segment path with valid offset
	validSegmentPath := filePath + "::0"
	data, err = readValidationInput(validSegmentPath)
	require.NoError(t, err)
	assert.Equal(t, payload, string(data))

	// Non-existent path
	_, err = readValidationInput(filepath.Join(tempDir, "missing.json"))
	assert.Error(t, err)
}

func TestCycle127_ConfigsAndCloning(t *testing.T) {
	setupIsolatedTestEnv(t)

	// cloneStringSliceMap
	assert.Nil(t, cloneStringSliceMap(nil))
	originalMap := map[string][]string{"k": {"v1", "v2"}}
	clonedMap := cloneStringSliceMap(originalMap)
	assert.Equal(t, originalMap, clonedMap)

	// cloneIDPrefixesConfig
	assert.Nil(t, cloneIDPrefixesConfig(nil))
	idCfg := &IDPrefixesConfig{
		KindToPrefixes: map[string][]string{"goal": {"GOL-"}},
		InferenceRules: InferenceRulesConfig{
			Patterns: []InferencePattern{{Pattern: ".*", PrefixTemplate: "GOL-"}},
			SynonymPatterns: []SynonymInferencePattern{
				{Pattern: ".*", Strategies: []string{"exact"}},
			},
		},
	}
	clonedIDCfg := cloneIDPrefixesConfig(idCfg)
	assert.Equal(t, idCfg.KindToPrefixes, clonedIDCfg.KindToPrefixes)

	// cloneNamespacesConfig
	assert.Nil(t, cloneNamespacesConfig(nil))
	nsCfg := &NamespacesConfig{
		NamespaceLayers: []string{"zqk", "domain"},
		Namespaces: map[string]NamespaceConfig{
			"default": {Kinds: []string{"goal"}, SubordinateNamespaces: []string{"sub"}},
		},
	}
	clonedNSCfg := cloneNamespacesConfig(nsCfg)
	assert.Equal(t, nsCfg.NamespaceLayers, clonedNSCfg.NamespaceLayers)

	// clonePathsConfig, cloneValidationTimeoutConfig, cloneValidationTierConfig
	assert.Nil(t, clonePathsConfig(nil))
	assert.Nil(t, cloneValidationTimeoutConfig(nil))
	assert.Nil(t, cloneValidationTierConfig(nil))

	assert.NotNil(t, clonePathsConfig(&PathsConfig{}))
	assert.NotNil(t, cloneValidationTimeoutConfig(&ValidationTimeoutConfig{}))
	assert.NotNil(t, cloneValidationTierConfig(&ValidationTierConfig{}))
}

func TestCycle127_ConfigLoadersAndFallbacks(t *testing.T) {
	tempDir := setupIsolatedTestEnv(t)

	// ID prefixes config: missing file, invalid yaml, valid file
	missingFile := filepath.Join(tempDir, "missing.yaml")
	_, err := LoadIDPrefixesConfig(missingFile)
	assert.Error(t, err)

	invalidFile := filepath.Join(tempDir, "invalid.yaml")
	err = fileutil.WriteFile(invalidFile, []byte("invalid: yaml: ["), 0644)
	require.NoError(t, err)
	_, err = LoadIDPrefixesConfig(invalidFile)
	assert.Error(t, err)

	globalCfg := GetGlobalIDPrefixesConfig()
	assert.NotNil(t, globalCfg)

	// Namespaces config: missing file, invalid yaml, GetGlobal
	nsMissing, err := LoadNamespacesConfig(missingFile)
	require.NoError(t, err)
	assert.NotNil(t, nsMissing)

	nsInvalid, err := LoadNamespacesConfig(invalidFile)
	require.NoError(t, err)
	assert.NotNil(t, nsInvalid)

	globalNS := GetGlobalNamespacesConfig()
	assert.NotNil(t, globalNS)
	assert.NotEmpty(t, globalNS.GetSystemOrigin())
	assert.NotEmpty(t, globalNS.GetProjectOrigin())
	assert.NotEmpty(t, globalNS.GetNamespaceValidationPattern())

	// Tier config: missing and invalid
	tierMissing, err := LoadValidationTierConfig(missingFile)
	require.NoError(t, err)
	assert.NotNil(t, tierMissing)

	// Timeout config: missing and invalid
	timeoutMissing, err := LoadValidationTimeoutConfig(missingFile)
	require.NoError(t, err)
	assert.NotNil(t, timeoutMissing)
}

func TestCycle127_EffortWallclock_ParsingAndFormatting(t *testing.T) {
	// ParseEffortHours qualitative tokens
	for _, tok := range []string{"small", "medium", "large", "xl", "xs", "tiny", "huge"} {
		_, ok := ParseEffortHours(tok)
		assert.False(t, ok, "token %s should return false", tok)
	}

	// Ranges
	h, ok := ParseEffortHours("2-4h")
	assert.True(t, ok)
	assert.Equal(t, 4.0, h)

	h, ok = ParseEffortHours("1 - 2 days")
	assert.True(t, ok)
	assert.Equal(t, 48.0, h)

	// Units: m, h, d, w
	h, ok = ParseEffortHours("30m")
	assert.True(t, ok)
	assert.Equal(t, 0.5, h)

	h, ok = ParseEffortHours("8h")
	assert.True(t, ok)
	assert.Equal(t, 8.0, h)

	h, ok = ParseEffortHours("2d")
	assert.True(t, ok)
	assert.Equal(t, 48.0, h)

	h, ok = ParseEffortHours("1w")
	assert.True(t, ok)
	assert.Equal(t, 168.0, h)

	// Formatting
	assert.Equal(t, "0h", FormatEffortHours(-5))
	assert.Equal(t, "1h", FormatEffortHours(1.005))
	assert.Equal(t, "1.50h", FormatEffortHours(1.5))

	assert.Equal(t, "0h", formatEffortHoursFloor(-1))
	assert.Equal(t, "0.50h", formatEffortHoursFloor(0.5))
	assert.Equal(t, "4h", formatEffortHoursFloor(4.9))
}

func TestCycle127_EffortWallclock_ClampingAndPolicy(t *testing.T) {
	now := time.Now()
	startTime := now.Add(-2 * time.Hour).Format(time.RFC3339)
	endTime := now.Format(time.RFC3339)
	invertedEndTime := now.Add(-4 * time.Hour).Format(time.RFC3339)

	// Nil object
	clamped, _, _ := ClampActualEffortToWallClock(nil)
	assert.False(t, clamped)
	assert.Nil(t, ValidateActualEffortWithinWallClock(nil))

	// Inverted span
	objInverted := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyStartedAt:    startTime,
		objects.FieldKeyCompletedAt:  invertedEndTime,
		objects.FieldKeyActualEffort: "5h",
	}
	clamped, prev, next := ClampActualEffortToWallClock(objInverted)
	assert.True(t, clamped)
	assert.Equal(t, "5h", prev)
	assert.Equal(t, "0h", next)

	// Normal span exceeded: 10h actual vs 2h wall
	objExceeded := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyStartedAt:    startTime,
		objects.FieldKeyCompletedAt:  endTime,
		objects.FieldKeyActualEffort: "10h",
	}
	clamped, _, next = ClampActualEffortToWallClock(objExceeded)
	assert.True(t, clamped)
	assert.Equal(t, "2h", next)

	// WallClockActualEffortString
	str, ok := WallClockActualEffortString(objExceeded)
	assert.True(t, ok)
	assert.NotEmpty(t, str)

	// ValidateActualEffortWithinWallClock
	objWarn := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyStartedAt:    startTime,
		objects.FieldKeyCompletedAt:  endTime,
		objects.FieldKeyActualEffort: "10h",
	}
	warnings := ValidateActualEffortWithinWallClock(objWarn)
	assert.Len(t, warnings, 1)

	// applyWorkEnvelopeWallClockPolicy
	assert.Nil(t, applyWorkEnvelopeWallClockPolicy(true, objWarn))
	assert.Nil(t, applyWorkEnvelopeWallClockPolicy(false, nil))
	assert.Nil(t, applyWorkEnvelopeWallClockPolicy(false, objWarn))
}

func TestCycle127_BucketingStrategy_Validation(t *testing.T) {
	// Missing strategy_type
	errs := ValidateBucketingStrategy(map[string]any{})
	require.NotEmpty(t, errs)
	assert.Equal(t, "strategy_type", errs[0].Field)

	// Invalid strategy_type
	errs = ValidateBucketingStrategy(map[string]any{"strategy_type": "invalid_type"})
	require.NotEmpty(t, errs)
	assert.Equal(t, "strategy_type", errs[0].Field)

	// Missing field
	errs = ValidateBucketingStrategy(map[string]any{"strategy_type": "state"})
	require.NotEmpty(t, errs)
	assert.Equal(t, "field", errs[0].Field)

	// Chronological missing format
	errs = ValidateBucketingStrategy(map[string]any{
		"strategy_type": "chronological",
		"field":         "created_at",
	})
	require.NotEmpty(t, errs)
	assert.Equal(t, "format", errs[0].Field)

	// Enabled true but applies_to empty
	errs = ValidateBucketingStrategy(map[string]any{
		"strategy_type": "state",
		"field":         "status",
		"enabled":       true,
		"applies_to":    []any{},
	})
	require.NotEmpty(t, errs)
	assert.Equal(t, "enabled", errs[0].Field)

	// Single tier archive missing archive_after
	errs = ValidateBucketingStrategy(map[string]any{
		"strategy_type": "state",
		"field":         "status",
		"archive_strategy": map[string]any{
			"enabled": true,
		},
	})
	require.NotEmpty(t, errs)
	assert.Equal(t, "archive_strategy.archive_after", errs[0].Field)

	// Multi tier archive invalid tier duration
	errs = ValidateBucketingStrategy(map[string]any{
		"strategy_type": "state",
		"field":         "status",
		"archive_strategy": map[string]any{
			"enabled": true,
			"tier_progression": []any{
				map[string]any{"tier": "invalid_tier", "duration": "bad_dur"},
			},
		},
	})
	require.NotEmpty(t, errs)

	// isValidDuration helper tests
	assert.True(t, isValidDuration("0"))
	assert.True(t, isValidDuration("720h"))
	assert.True(t, isValidDuration("30d"))
	assert.False(t, isValidDuration(""))
	assert.False(t, isValidDuration("30x"))
}

func TestCycle127_TypeValidator_And_MembershipHelpers(t *testing.T) {
	// validateDateTimeType
	assert.True(t, validateDateTimeType("2026-10-23T12:00:00Z"))
	assert.False(t, validateDateTimeType("invalid-date"))
	assert.True(t, validateDateTimeType(time.Now()))
	assert.False(t, validateDateTimeType(struct{ Foo string }{Foo: "bar"}))
	assert.False(t, validateDateTimeType(12345))

	// validateDateType, validateObjectType, validateTextType
	assert.True(t, validateDateType("2026-10-23"))
	assert.False(t, validateDateType("2026/10/23"))
	assert.True(t, validateObjectType(map[string]any{}))
	assert.False(t, validateObjectType("not-a-map"))
	assert.True(t, validateTextType("text"))
	assert.False(t, validateTextType(123))

	// Plan membership helpers
	assert.True(t, planStatusRequiresReadyChildren(objects.ObjectStatusInProgress))
	assert.False(t, planStatusRequiresReadyChildren(objects.ObjectStatusDraft))

	assert.True(t, BacklogItemStatusReadyOrLater(objects.ObjectStatusPlanned))
	assert.False(t, BacklogItemStatusReadyOrLater(objects.ObjectStatusDraft))

	assert.True(t, backlogItemAllowedOnExecutionFacingPlan(objects.ObjectStatusPlanned))
	assert.True(t, backlogItemAllowedOnExecutionFacingPlan(objects.ObjectStatusError))

	assert.True(t, BacklogItemStatusTerminalForPlanCompletion(objects.ObjectStatusComplete))
	assert.False(t, BacklogItemStatusTerminalForPlanCompletion(objects.ObjectStatusInProgress))

	assert.True(t, backlogDependentStillReferencesPlan("PRI-001", "BLI-001", nil))
	assert.False(t, iterateLinkedBacklogItems("", nil, nil, true, nil))
}

func TestCycle127_IDValidator_PrefixesAndFormat(t *testing.T) {
	setupIsolatedTestEnv(t)
	validator := GetIDValidator()

	// inferPrefixesFromKind fallback checks
	assert.Equal(t, []string{"ASK-"}, validator.inferPrefixesFromKind("agent_skill"))
	assert.Equal(t, []string{"ATK-"}, validator.inferPrefixesFromKind("agent_task"))
	assert.Equal(t, []string{"PER-"}, validator.inferPrefixesFromKind("persona"))

	// ReloadPatterns
	err := validator.ReloadPatterns()
	require.NoError(t, err)

	// validateColonFormat
	err = validator.validateColonFormat("foo:bar:baz")
	assert.Error(t, err)

	err = validator.validateColonFormat("single")
	require.NoError(t, err)

	// validateNamespaceFormat
	err = validator.validateNamespaceFormat(&ParsedNamespace{
		NamespaceID: "invalid!!ns",
	})
	assert.Error(t, err)
}

func TestCycle127_ValidationMetrics_Comprehensive(t *testing.T) {
	tempDir := setupIsolatedTestEnv(t)
	vm := NewValidationMetrics()

	vm.RecordEnqueue(20 * time.Millisecond)
	vm.RecordValidation(100 * time.Millisecond)
	vm.RecordCollection(10 * time.Millisecond)
	vm.IncrementValidated()
	vm.IncrementFailed()
	vm.IncrementCacheHit()
	vm.IncrementCacheMiss()
	vm.IncrementRetry()

	vm.SetWorkerCount(4)
	vm.UpdateQueueSize(5)
	vm.UpdateQueueSize(15)

	vm.RecordTierIssue(1)
	vm.RecordTierIssue(2)
	vm.RecordTierIssue(3)
	vm.RecordTierIssue(4)

	vm.RecordLockWait(60 * time.Millisecond)
	vm.RecordLockHold(10 * time.Millisecond)
	vm.RecordLockTimeout()
	vm.RecordHashRegistryLoad(15 * time.Millisecond)
	vm.IncrementHashRegistryConcurrentAccess()
	vm.RecordHashRegistryOperations(2)
	vm.RecordHashRegistryBucketedProcessing(25 * time.Millisecond)
	vm.RecordHashRegistryNonBucketedProcessing(15 * time.Millisecond)

	assert.GreaterOrEqual(t, vm.GetObjectsPerSecond(), 0.0)
	assert.GreaterOrEqual(t, vm.GetContentionRate(), 0.0)
	assert.GreaterOrEqual(t, vm.GetTimeoutRate(), 0.0)
	assert.Greater(t, vm.GetMaxWaitTime(), time.Duration(0))

	vm.Finalize()
	assert.NotEmpty(t, vm.String())

	data, err := vm.JSONSnapshot()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	metricsFile := filepath.Join(tempDir, "metrics.json")
	err = vm.Save(metricsFile)
	require.NoError(t, err)
}

func TestCycle127_OutputQueue_SyncBufferAndHandler(t *testing.T) {
	setupIsolatedTestEnv(t)
	buf := &bytes.Buffer{}
	sb := newSyncBuffer(buf)

	n, err := sb.WriteString("hello ")
	require.NoError(t, err)
	assert.Equal(t, 6, n)

	n, err = sb.Write([]byte("world\n"))
	require.NoError(t, err)
	assert.Equal(t, 6, n)

	err = sb.Flush()
	require.NoError(t, err)
	assert.Equal(t, "hello world\n", buf.String())

	// WriterOutputHandler with stdout/stderr (safe close)
	stdoutHandler := &WriterOutputHandler{writer: os.Stdout}
	err = stdoutHandler.Close()
	require.NoError(t, err)

	// WriterOutputHandler with flusher
	memBuf := &bytes.Buffer{}
	customHandler := &WriterOutputHandler{writer: memBuf}
	err = customHandler.Write("line1\n")
	require.NoError(t, err)
	err = customHandler.Write([]byte("line2\n"))
	require.NoError(t, err)
	err = customHandler.Write(100)
	require.NoError(t, err)
	err = customHandler.Flush()
	require.NoError(t, err)
	err = customHandler.Close()
	require.NoError(t, err)
	assert.Contains(t, memBuf.String(), "line1")
}

func TestCycle127_StateCache_LoadVariants(t *testing.T) {
	tempDir := setupIsolatedTestEnv(t)
	cacheFile := filepath.Join(tempDir, "validation_cache.json")

	// Case 1: suppressLoadUntilSave
	cacheSuppress := &ValidationStateCache{
		cacheFile:             cacheFile,
		suppressLoadUntilSave: true,
		cache:                 make(map[string]*ValidationState),
	}
	err := cacheSuppress.Load()
	require.NoError(t, err)

	// Case 2: Legacy States format
	checksum := computeValidationCodeChecksum(tempDir)
	legacyStates := map[string]any{
		"code_checksum": checksum,
		"states": []map[string]any{
			{
				"object_id":      "GOL-LEGACY-1",
				"object_kind":    objects.KindGoal,
				"last_validated": time.Now().Format(time.RFC3339),
			},
		},
		"updated": time.Now().Format(time.RFC3339),
	}
	data, err := json.Marshal(legacyStates)
	require.NoError(t, err)
	err = fileutil.WriteFile(cacheFile, data, 0644)
	require.NoError(t, err)

	cacheLegacy := &ValidationStateCache{
		cacheFile:   cacheFile,
		projectRoot: tempDir,
		cache:       make(map[string]*ValidationState),
	}
	err = cacheLegacy.Load()
	require.NoError(t, err)
	assert.Contains(t, cacheLegacy.cache, "GOL-LEGACY-1")

	// Case 3: Legacy ByKind raw array format
	legacyByKind := map[string]any{
		"code_checksum": checksum,
		"by_kind": map[string]any{
			"goal": []map[string]any{
				{
					"id":             "GOL-BYKIND-1",
					"path":           "goals/g1.yaml",
					"last_validated": time.Now().Format(time.RFC3339),
				},
			},
		},
	}
	data, err = json.Marshal(legacyByKind)
	require.NoError(t, err)
	err = fileutil.WriteFile(cacheFile, data, 0644)
	require.NoError(t, err)

	cacheByKind := &ValidationStateCache{
		cacheFile:   cacheFile,
		projectRoot: tempDir,
		cache:       make(map[string]*ValidationState),
	}
	err = cacheByKind.Load()
	require.NoError(t, err)
	assert.Contains(t, cacheByKind.cache, "GOL-BYKIND-1")
}

func TestCycle127_GoValidator_DynamicRules(t *testing.T) {
	setupIsolatedTestEnv(t)
	gv := NewGoValidator()

	gv.dynamicRulesCache.Store("goal:active", []map[string]any{
		{
			objects.FieldKeyID:             "RULE-PRESENCE-1",
			objects.FieldKeyRuleType:       "field_presence",
			objects.FieldKeyTransitionFrom: "draft",
			objects.FieldKeyParameters: map[string]any{
				objects.FieldKeyFieldName: "description",
			},
		},
		{
			objects.FieldKeyID:       "RULE-ACTIVE-REF-1",
			objects.FieldKeyRuleType: "active_reference",
			objects.FieldKeyParameters: map[string]any{
				"reference_field": "vision_ref",
			},
		},
	})

	// Missing description -> produces error
	obj := map[string]any{
		"vision_ref": "VIS-001",
	}
	opts := &ValidationOptions{
		ObjectStatusLookup: func(id string) (string, error) {
			return objects.ObjectStatusInProgress, nil
		},
	}
	errs := gv.validateDynamicRules("goal", "active", "draft", obj, opts)
	assert.NotEmpty(t, errs)

	// Transition from non-matching skips rule
	errsSkipped := gv.validateDynamicRules("goal", "active", "completed", obj, opts)
	assert.Empty(t, errsSkipped)
}

type cycle127MockExtHandler struct {
	kind string
	errs []ValidationError
}

func (m *cycle127MockExtHandler) Kind() string { return m.kind }

func (m *cycle127MockExtHandler) Validate(ctx context.Context, obj map[string]any, opts *ValidationOptions) []ValidationError {
	return m.errs
}

func TestCycle127_ExtendedValidationRegistry_Comprehensive(t *testing.T) {
	setupIsolatedTestEnv(t)
	reg := NewExtendedValidationRegistry()
	assert.NotNil(t, reg)

	// Nil receiver and nil obj
	var nilReg *ExtendedValidationRegistry
	assert.Nil(t, nilReg.Validate(context.Background(), map[string]any{}, "", nil))
	assert.Nil(t, reg.Validate(context.Background(), nil, "", nil))

	// EventBus operations
	bus := reg.EventBus()
	assert.NotNil(t, bus)
	bus.Subscribe(nil)
	var captured ValidationEvent
	bus.Subscribe(func(ev ValidationEvent) {
		captured = ev
	})
	bus.Publish(ValidationEvent{Type: ValidationEventPassed, Kind: objects.KindGoal})
	assert.Equal(t, ValidationEventPassed, captured.Type)

	// Register handlers: nil, universal, and kind-specific
	reg.Register(nil)
	reg.Register(&cycle127MockExtHandler{kind: KindUniversal})
	reg.Register(&cycle127MockExtHandler{
		kind: objects.KindGoal,
		errs: []ValidationError{{Field: "title", Message: "title required"}},
	})
	reg.SetPool(NewValidationPool(2))

	// Validate with multiple handlers
	objGoal := map[string]any{
		objects.FieldKeyID:   "GOL-EXT-1",
		objects.FieldKeyKind: objects.KindGoal,
	}
	errs := reg.Validate(context.Background(), objGoal, "", nil)
	assert.Len(t, errs, 1)

	// Validate with canceled context
	cancCtx, cancel := context.WithCancel(context.Background())
	cancel()
	cancErrs := reg.Validate(cancCtx, objGoal, "", nil)
	assert.NotEmpty(t, cancErrs)
}

func TestCycle127_OverlayDSL_FieldHelpersAndLinkBack(t *testing.T) {
	setupIsolatedTestEnv(t)

	// isFieldNonEmpty
	assert.False(t, isFieldNonEmpty(nil, "f"))
	assert.False(t, isFieldNonEmpty(map[string]any{}, "f"))
	assert.False(t, isFieldNonEmpty(map[string]any{"f": "  "}, "f"))
	assert.True(t, isFieldNonEmpty(map[string]any{"f": "val"}, "f"))
	assert.False(t, isFieldNonEmpty(map[string]any{"f": []any{}}, "f"))
	assert.False(t, isFieldNonEmpty(map[string]any{"f": []any{""}}, "f"))
	assert.True(t, isFieldNonEmpty(map[string]any{"f": []any{"item"}}, "f"))
	assert.True(t, isFieldNonEmpty(map[string]any{"f": []any{123}}, "f"))
	assert.False(t, isFieldNonEmpty(map[string]any{"f": map[string]any{}}, "f"))
	assert.True(t, isFieldNonEmpty(map[string]any{"f": map[string]any{"k": "v"}}, "f"))
	assert.True(t, isFieldNonEmpty(map[string]any{"f": 42}, "f"))

	// isFieldCleared
	assert.True(t, isFieldCleared(nil, "f"))
	assert.True(t, isFieldCleared(map[string]any{}, "f"))
	assert.True(t, isFieldCleared(map[string]any{"f": ""}, "f"))
	assert.False(t, isFieldCleared(map[string]any{"f": "val"}, "f"))
	assert.True(t, isFieldCleared(map[string]any{"f": []any{}}, "f"))
	assert.False(t, isFieldCleared(map[string]any{"f": []any{"x"}}, "f"))
	assert.True(t, isFieldCleared(map[string]any{"f": map[string]any{}}, "f"))
	assert.False(t, isFieldCleared(map[string]any{"f": map[string]any{"k": "v"}}, "f"))

	// evalAnyNonEmpty
	assert.False(t, evalAnyNonEmpty(map[string]any{}, nil))
	assert.False(t, evalAnyNonEmpty(map[string]any{}, []string{""}))
	assert.True(t, evalAnyNonEmpty(map[string]any{"item": "val"}, []string{"items"}))
	assert.True(t, evalAnyNonEmpty(map[string]any{"items": []any{"x"}}, []string{"item"}))

	// checkContentSizeMeasured
	assert.False(t, checkContentSizeMeasured(map[string]any{}))
	assert.True(t, checkContentSizeMeasured(map[string]any{"content_size": 10}))
	assert.True(t, checkContentSizeMeasured(map[string]any{"content_size": int64(20)}))
	assert.True(t, checkContentSizeMeasured(map[string]any{"content_size": 30.5}))
	assert.False(t, checkContentSizeMeasured(map[string]any{"content_size": "not-int"}))

	// Workflow stages
	gv := NewGoValidator()
	objDispatch := map[string]any{
		"persona_refs": []any{"PER-1"},
	}
	h, m := evalOverlayWorkflowStage(gv, objDispatch, nil, "team_or_persona_dispatch_refs", "")
	assert.True(t, h)
	assert.True(t, m)

	h, _ = evalOverlayWorkflowStage(gv, objDispatch, nil, "active_ref", "goal_ref")
	assert.True(t, h)

	h, _ = evalOverlayWorkflowStage(gv, objDispatch, nil, "link_back", "goal_ref:vision_ref")
	assert.True(t, h)

	h, _ = evalOverlayWorkflowStage(gv, map[string]any{objects.FieldKeyID: "PRI-1"}, nil, "linked_backlog_ready_or_later", "")
	assert.True(t, h)

	h, _ = evalOverlayWorkflowStage(gv, map[string]any{objects.FieldKeyID: "PRI-1"}, nil, "linked_backlog_all_terminal", "")
	assert.True(t, h)

	h, _ = evalOverlayWorkflowStage(gv, map[string]any{objects.FieldKeyID: "PRI-1"}, nil, "no_linked_backlog_in_progress_or_complete", "")
	assert.True(t, h)
}

func TestCycle127_AsyncValidatorStopPipeline_Stages(t *testing.T) {
	tempDir := setupIsolatedTestEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	av := NewAsyncValidator(ctx, tempDir, 1, time.Minute)
	defer av.Stop()

	err := runAsyncValidatorStopPipeline(10*time.Millisecond, 10*time.Millisecond, av)
	require.NoError(t, err)
}

func TestCycle127_NamespaceRegistry_Operations(t *testing.T) {
	tempDir := setupIsolatedTestEnv(t)
	nr := NewNamespaceRegistry(tempDir)
	assert.NotNil(t, nr)

	// RegisterNamespace and GetAllNamespaces
	nr.RegisterNamespace("custom_kind", "domain:custom")
	all := nr.GetAllNamespaces()
	assert.Equal(t, "domain:custom", all["custom_kind"])

	// Subordinate operations
	assert.True(t, nr.IsSubordinateOf("zqk:kernel:sub", "zqk:kernel"))
	assert.False(t, nr.IsSubordinateOf("domain:finance", "zqk:kernel"))
	assert.Equal(t, "zqk:kernel", nr.GetParentNamespace("zqk:kernel:sub"))
	assert.Equal(t, "", nr.GetParentNamespace("single"))

	// GetSubordinateNamespaces and GetNamespaceForKind
	subs := nr.GetSubordinateNamespaces("zqk:kernel")
	assert.NotNil(t, subs)

	ns := nr.GetNamespaceForKind("goal")
	assert.NotEmpty(t, ns)
	nsUnknown := nr.GetNamespaceForKind("unknown_random_kind")
	assert.NotEmpty(t, nsUnknown)
}

func TestCycle127_NamespacesConfig_InferenceAndValidation(t *testing.T) {
	setupIsolatedTestEnv(t)
	cfg := &NamespacesConfig{
		DefaultNamespace: "zqk:kernel",
		Namespaces: map[string]NamespaceConfig{
			"domain:sales": {Kinds: []string{"lead", "deal"}},
		},
		InferenceRules: []NamespaceInferenceRule{
			{Pattern: "^lead$", Namespace: "domain:sales"},
			{Pattern: "^(.*)_integration$", NamespaceTemplate: "integration:{domain}"},
			{Pattern: "[invalid-regex", Namespace: "domain:invalid"},
		},
	}

	// Explicit kind mapping
	assert.Equal(t, "domain:sales", cfg.GetNamespaceForKind("deal"))

	// Inference rule with static namespace
	assert.Equal(t, "domain:sales", cfg.GetNamespaceForKind("lead"))

	// Inference rule with {domain} template
	assert.Equal(t, "integration:github", cfg.GetNamespaceForKind("github_integration"))

	// Invalid regex rule skipped, fallback to default
	assert.Equal(t, "zqk:kernel", cfg.GetNamespaceForKind("other_unmatched"))

	// validate() defaults
	cfgEmpty := &NamespacesConfig{}
	cfgEmpty.validate()
	assert.NotEmpty(t, cfgEmpty.DefaultNamespace)
	assert.NotEmpty(t, cfgEmpty.NamespaceLayers)
	assert.NotEmpty(t, cfgEmpty.NamespaceValidationPattern)
}

func TestCycle127_IDValidator_NamespaceEdgeCases(t *testing.T) {
	setupIsolatedTestEnv(t)
	validator := GetIDValidator()

	// Parsed layer without namespace ID -> error
	err := validator.validateNamespaceFormat(&ParsedNamespace{
		Layer: "unknown_layer",
	})
	assert.Error(t, err)

	// Colon format with 3 parts and unknown layer -> error
	err = validator.validateColonFormat("unknown_layer:sub:item")
	assert.Error(t, err)
}

func TestCycle127_UniversalQualityValidationHandler(t *testing.T) {
	setupIsolatedTestEnv(t)
	handler := NewUniversalQualityValidationHandler()
	assert.Equal(t, KindUniversal, handler.Kind())

	// Nil object returns nil
	assert.Nil(t, handler.Validate(context.Background(), nil, nil))

	// Criteria missing category
	objCrit := map[string]any{
		objects.FieldKeyKind: objects.KindCriteria,
	}
	errs := handler.Validate(context.Background(), objCrit, nil)
	assert.NotEmpty(t, errs)

	// Criteria invalid category
	objCritInvalidCat := map[string]any{
		objects.FieldKeyKind:     objects.KindCriteria,
		objects.FieldKeyCategory: "non_existent_category",
	}
	errs = handler.Validate(context.Background(), objCritInvalidCat, nil)
	assert.NotEmpty(t, errs)

	// Short title, multiline title, placeholder title
	objShortTitle := map[string]any{
		objects.FieldKeyKind:  objects.KindGoal,
		objects.FieldKeyTitle: "abc",
	}
	errs = handler.Validate(context.Background(), objShortTitle, nil)
	assert.NotEmpty(t, errs)

	objMultilineTitle := map[string]any{
		objects.FieldKeyKind:  objects.KindGoal,
		objects.FieldKeyTitle: "First line\nSecond line",
	}
	errs = handler.Validate(context.Background(), objMultilineTitle, nil)
	assert.NotEmpty(t, errs)

	objPlaceholderTitle := map[string]any{
		objects.FieldKeyKind:  objects.KindGoal,
		objects.FieldKeyTitle: "TODO",
	}
	errs = handler.Validate(context.Background(), objPlaceholderTitle, nil)
	assert.NotEmpty(t, errs)

	// Valid criteria
	objValid := map[string]any{
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyCategory:    "functional",
		objects.FieldKeyTitle:       "Valid and Substantive Title",
		objects.FieldKeyDescription: "This is a detailed and substantive description for testing.",
	}
	errs = handler.Validate(context.Background(), objValid, nil)
	assert.Empty(t, errs)

	// Refactor criteria with completeness validation
	objRefactor := map[string]any{
		objects.FieldKeyKind:                   objects.KindCriteria,
		objects.FieldKeyCategory:               "functional",
		objects.FieldKeyTitle:                  "Refactor the authentication module",
		objects.FieldKeyDescription:            "Substantive description for refactoring the module cleanly.",
		objects.FieldKeyCompletenessValidation: []string{"invalid [[ predicate", "assert count > 0"},
	}
	errsRefactor := handler.Validate(context.Background(), objRefactor, nil)
	assert.NotEmpty(t, errsRefactor)

	// Persona handler
	personaHandler := NewPersonaExtendedValidationHandler()
	assert.Equal(t, objects.KindPersona, personaHandler.Kind())
	assert.Nil(t, personaHandler.Validate(context.Background(), nil, nil))
	assert.Nil(t, personaHandler.Validate(context.Background(), map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusDraft,
	}, nil))
	errsPersona := personaHandler.Validate(context.Background(), map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusApproved,
	}, nil)
	assert.NotEmpty(t, errsPersona)
}

func TestCycle127_IDPrefixesConfig_InferenceHelpers(t *testing.T) {
	setupIsolatedTestEnv(t)

	// matchPattern
	m, err := matchPattern("*_item", "test_item")
	require.NoError(t, err)
	assert.True(t, m)

	m, err = matchPattern("user_*", "user_profile")
	require.NoError(t, err)
	assert.True(t, m)

	m, err = matchPattern("exact", "exact")
	require.NoError(t, err)
	assert.True(t, m)

	m, err = matchPattern("exact", "other")
	require.NoError(t, err)
	assert.False(t, m)

	// inferSynonymsFromKind with strategies
	cfg := &IDPrefixesConfig{
		InferenceRules: InferenceRulesConfig{
			SynonymPatterns: []SynonymInferencePattern{
				{
					Pattern:    "*_item",
					Strategies: []string{"first_letters", "two_word_abbrev"},
				},
				{
					Pattern:    "single",
					Strategies: []string{"first_4"},
				},
			},
		},
	}
	synonyms := cfg.inferSynonymsFromKind("backlog_item")
	assert.Contains(t, synonyms, "bi")

	synSingle := cfg.inferSynonymsFromKind("single")
	assert.Contains(t, synSingle, "sing")

	// inferPrefixFromKind
	assert.Equal(t, []string{"TES-"}, cfg.inferPrefixFromKind("test_item"))
	assert.Equal(t, []string{"SYS-"}, cfg.inferPrefixFromKind("system_metric"))
	assert.Equal(t, []string{"COM-"}, cfg.inferPrefixFromKind("compound_word"))
	assert.Equal(t, []string{"ABC-"}, inferPrefixFromFirstPart("abc_def"))
	assert.Empty(t, inferPrefixFromFirstPart("ab"))
}

func TestCycle127_OverlayDSL_FieldStages(t *testing.T) {
	setupIsolatedTestEnv(t)
	gv := NewGoValidator()

	// evalOverlayDocStage
	h, m := evalOverlayDocStage(map[string]any{objects.FieldKeyID: "GOL-1"}, "standard_checks_pass")
	assert.True(t, h)
	assert.True(t, m)

	h, _ = evalOverlayDocStage(map[string]any{objects.FieldKeyPath: "valid.yaml"}, "path_exists")
	assert.True(t, h)

	h, _ = evalOverlayDocStage(map[string]any{"content_hash": "hash123"}, "content_hash_matches")
	assert.True(t, h)

	h, m = evalOverlayDocStage(map[string]any{"content_hash": ""}, "content_hash_matches")
	assert.True(t, h)
	assert.False(t, m)

	h, m = evalOverlayDocStage(map[string]any{"content_size": 50}, "content_size_positive")
	assert.True(t, h)
	assert.True(t, m)

	// evalOverlayFieldStage: field_nonempty, field_cleared, any_nonempty, field_matches, title_body_cohesion
	obj := map[string]any{
		"status":      "planned",
		"active_flag": "true",
		"title":       "Refactor authentication token parser",
		"description": "This refactors the token parser to prevent regressions and improve security.",
	}
	h, m = evalOverlayFieldStage(obj, "field_nonempty", "status", gv)
	assert.True(t, h)
	assert.True(t, m)

	h, m = evalOverlayFieldStage(obj, "field_cleared", "empty_field", gv)
	assert.True(t, h)
	assert.True(t, m)

	h, m = evalOverlayFieldStage(obj, "any_nonempty", "status,missing", gv)
	assert.True(t, h)
	assert.True(t, m)

	h, m = evalOverlayFieldStage(obj, "field_matches", "status:^plan", gv)
	assert.True(t, h)
	assert.True(t, m)

	h, _ = evalOverlayFieldStage(obj, "title_body_cohesion", "1", gv)
	assert.True(t, h)
}

func TestCycle127_OutputWriter_PipelineAndBatching(t *testing.T) {
	setupIsolatedTestEnv(t)
	queue := NewOutputQueue(100)
	ctx := context.Background()

	writer := NewOutputWriter(ctx, queue, nil)
	memBuf := &bytes.Buffer{}
	handler := &WriterOutputHandler{writer: memBuf}

	writer.RegisterHandler("default", handler)
	writer.Start()

	// Enqueue packets with handler and without handler
	queue.Enqueue(OutputPacket{
		ChannelID: "default",
		Data:      "item1\n",
		FlushHint: true,
	})
	queue.Enqueue(OutputPacket{
		ChannelID: "unregistered",
		Data:      "item2\n",
	})

	time.Sleep(20 * time.Millisecond)
	err := writer.Stop()
	require.NoError(t, err)
	assert.Contains(t, memBuf.String(), "item1")
}

func TestCycle127_AsyncValidator_DrainAndHex(t *testing.T) {
	tempDir := setupIsolatedTestEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	av := NewAsyncValidator(ctx, tempDir, 1, time.Minute)
	defer av.Stop()

	// Test IsHexString
	assert.True(t, IsHexString("1234567890abcdefABCDEF"))
	assert.False(t, IsHexString(""))
	assert.False(t, IsHexString("xyz!"))

	// Test Drain with active worker transition
	av.activeWorkers.Store(1)
	av.wg.Add(1)
	goroutinelabels.NewGoroutine("test_drain_worker", "simulating worker completion").StartSimple(func() {
		time.Sleep(10 * time.Millisecond)
		av.activeWorkers.Store(0)
		av.wg.Done()
	})

	err := av.Drain(context.Background())
	require.NoError(t, err)
}

func TestCycle127_FieldValidators_Normalization(t *testing.T) {
	setupIsolatedTestEnv(t)

	// String validation
	s, ok := ResolveStringForPatternValidation("hello")
	assert.True(t, ok)
	assert.Equal(t, "hello", s)

	// Schema version float formatting
	s, ok = ResolveStringForPatternValidationWithField("schema_version", 2.0)
	assert.True(t, ok)
	assert.Equal(t, "2.0.0", s)

	s, ok = ResolveStringForPatternValidationWithField("schema_version", -1.0)
	assert.True(t, ok)
	assert.Equal(t, "0.0.0", s)

	s, ok = ResolveStringForPatternValidationWithField("schema_version", 1500.0)
	assert.True(t, ok)
	assert.Equal(t, "999.99.0", s)

	// DateTime int64, int, float64
	ts := int64(1700000000)
	s, ok = ResolveStringForPatternValidationWithField("created_at", ts)
	assert.True(t, ok)
	assert.NotEmpty(t, s)

	s, ok = ResolveStringForPatternValidationWithField("created_at", int(ts))
	assert.True(t, ok)
	assert.NotEmpty(t, s)

	s, ok = ResolveStringForPatternValidationWithField("created_at", float64(ts))
	assert.True(t, ok)
	assert.NotEmpty(t, s)

	// DateTime string timestamp
	s, ok = ResolveStringForPatternValidationWithField("created_at", "1700000000")
	assert.True(t, ok)
	assert.NotEmpty(t, s)

	// Invalid type
	_, ok = ResolveStringForPatternValidationWithField("created_at", true)
	assert.False(t, ok)

	// isDateTimePatternField
	assert.True(t, isDateTimePatternField("created_at"))
	assert.True(t, isDateTimePatternField("updated_at"))
	assert.False(t, isDateTimePatternField("title"))
}

func TestCycle127_PathsAndTierTimeout_Helpers(t *testing.T) {
	setupIsolatedTestEnv(t)

	// PathsConfig helpers
	pathsCfg, err := LoadPathsConfig("")
	require.NoError(t, err)
	assert.NotNil(t, pathsCfg)

	globalPaths := GetGlobalPathsConfig()
	assert.NotNil(t, globalPaths)

	assert.Empty(t, pathsCfg.FindPath("non_existent_key"))
	ResetGlobalPathsConfig()

	// ValidationTierConfig helpers
	tierCfg, err := LoadValidationTierConfig("")
	require.NoError(t, err)
	assert.NotNil(t, tierCfg)

	globalTier := GetGlobalValidationTierConfig()
	assert.NotNil(t, globalTier)

	assert.Equal(t, 2, tierCfg.GetTierForRule("unknown_rule_127"))
	assert.True(t, tierCfg.IsBlockingTier(1))
	assert.False(t, tierCfg.IsBlockingTier(99))

	// ValidationTimeoutConfig helpers
	timeoutCfg, err := LoadValidationTimeoutConfig("")
	require.NoError(t, err)
	assert.NotNil(t, timeoutCfg)

	globalTimeout := GetGlobalValidationTimeoutConfig()
	assert.NotNil(t, globalTimeout)

	assert.Greater(t, timeoutCfg.StuckTimeout(), time.Duration(0))
	assert.Greater(t, timeoutCfg.TimeoutForKind("goal"), time.Duration(0))
	assert.Greater(t, timeoutCfg.TimeoutForKind("non_existent_kind"), time.Duration(0))
}

func TestCycle127_IDValidator_ParseSpecFile(t *testing.T) {
	tempDir := setupIsolatedTestEnv(t)
	validator := GetIDValidator()

	// Missing file -> error
	_, err := validator.parseSpecFile(filepath.Join(tempDir, "missing_spec.yaml"))
	assert.Error(t, err)

	// Empty spec without ontology or kind -> nil, nil
	emptySpecPath := filepath.Join(tempDir, "empty_spec.yaml")
	err = fileutil.WriteFile(emptySpecPath, []byte("title: No Ontology Spec"), 0644)
	require.NoError(t, err)
	cfg, err := validator.parseSpecFile(emptySpecPath)
	require.NoError(t, err)
	assert.Nil(t, cfg)

	// Spec with id_prefixes
	prefixSpecPath := filepath.Join(tempDir, "prefix_spec.yaml")
	prefixSpecContent := `
ontology: custom_widget
id_prefixes:
  - "WID-"
fields:
  id:
    validation:
      pattern: "^WID-[0-9]+$"
`
	err = fileutil.WriteFile(prefixSpecPath, []byte(prefixSpecContent), 0644)
	require.NoError(t, err)
	cfg, err = validator.parseSpecFile(prefixSpecPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "custom_widget", cfg.Kind)
	assert.Contains(t, cfg.Prefixes, "WID-")

	// Spec with id_template
	tmplSpecPath := filepath.Join(tempDir, "tmpl_spec.yaml")
	tmplSpecContent := `
kind: gadget
id_template: "GAD-{sequence}"
`
	err = fileutil.WriteFile(tmplSpecPath, []byte(tmplSpecContent), 0644)
	require.NoError(t, err)
	cfg, err = validator.parseSpecFile(tmplSpecPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "gadget", cfg.Kind)
	assert.Contains(t, cfg.Prefixes, "GAD-")
}

func TestCycle127_CoverageCushion_PathsAndID(t *testing.T) {
	setupIsolatedTestEnv(t)
	cfg := getDefaultPathsConfig()
	assert.NotEmpty(t, cfg.SearchStrategy.RelativePaths)
	assert.NotEmpty(t, cfg.SearchStrategy.Markers)
	assert.NotEmpty(t, cfg.Paths)

	val := GetIDValidator()
	ok, err := val.validateActualID("TST-12345", &IDPatternConfig{
		Prefixes: []string{"TST-"},
	})
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestCycle127_GoValidator_ConstructorsAndFeatureSupport(t *testing.T) {
	setupIsolatedTestEnv(t)
	gv := NewGoValidator()

	assert.Equal(t, "go", gv.Name())
	assert.True(t, gv.SupportsFeature("semantic_types"))
	assert.True(t, gv.SupportsFeature("custom_rules"))
	assert.False(t, gv.SupportsFeature("shacl_export"))
	assert.False(t, gv.SupportsFeature("unsupported_feature"))

	gvLoaders := NewGoValidatorWithLoaders(nil, nil)
	assert.NotNil(t, gvLoaders)

	gvID := NewGoValidatorWithIDValidator(nil, nil, nil)
	assert.NotNil(t, gvID)

	// Validate nil obj returns error
	_, err := gv.Validate(context.Background(), nil, objects.KindGoal, nil)
	assert.Error(t, err)

	// isDocumentationTitleProse
	assert.True(t, isDocumentationTitleProse("required at creation"))
	assert.True(t, isDocumentationTitleProse("none (required at creation)"))
	assert.True(t, isDocumentationTitleProse("required"))
	assert.True(t, isDocumentationTitleProse("optional"))
	assert.True(t, isDocumentationTitleProse("TBD"))
	assert.True(t, isDocumentationTitleProse("tbd"))
	assert.False(t, isDocumentationTitleProse("Proper Subsystem Title"))
}
