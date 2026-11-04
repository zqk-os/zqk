// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package systemcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

type mockLifecycleValidator struct {
	result *validation.ValidationResult
	err    error
}

func (m *mockLifecycleValidator) Validate(_ context.Context, _ map[string]any, _ string, _ *validation.ValidationOptions) (*validation.ValidationResult, error) {
	return m.result, m.err
}

func (m *mockLifecycleValidator) Name() string {
	return "mock_lifecycle"
}

func (m *mockLifecycleValidator) SupportsFeature(feature string) bool {
	return feature == "lifecycle"
}

type mockStorageWithError struct {
	storagepkg.ObjectStorageProvider
	failKind string
}

func (m *mockStorageWithError) List(
	_ context.Context,
	_ *pkgctx.SecurityContext,
	_ *pkgctx.StorageContext,
	filter storagepkg.ListFilter,
) (*storagepkg.QueryResult, error) {
	if filter.Kind == m.failKind {
		return nil, fmt.Errorf("simulated storage failure for %s", filter.Kind)
	}
	return &storagepkg.QueryResult{Objects: []map[string]any{}}, nil
}

func TestCheckRegistration_SeparatorsAndExtraText(t *testing.T) {
	separators := []string{" - extra", " -extra", "- extra"}
	for _, sep := range separators {
		obj := &parser.ParsedObject{
			ID:   "POL-DEFAULT-100",
			Kind: "policy" + sep,
			Properties: map[string]any{
				objects.FieldKeyCreatedBy: "ACC-100",
				objects.FieldKeyUpdatedBy: "ACC-100",
			},
		}
		issues := CheckRegistration(obj, "policy")
		require.NotEmpty(t, issues)
		var foundExtra bool
		for _, iss := range issues {
			if iss.Tier == 3 && iss.Category == "registration" {
				foundExtra = true
			}
		}
		assert.True(t, foundExtra, "expected Tier 3 extra text issue for separator: %s", sep)
	}

	cleanObj := &parser.ParsedObject{
		ID:   "POL-DEFAULT-200",
		Kind: "policy",
		Properties: map[string]any{
			objects.FieldKeyCreatedBy: "ACC-200",
			objects.FieldKeyUpdatedBy: "ACC-200",
		},
	}
	cleanIssues := CheckRegistration(cleanObj, "policy")
	assert.Empty(t, cleanIssues, "clean registration object should yield zero issues")
}

func TestCheckRegistration_KindMismatchAndDirResolution(t *testing.T) {
	objMismatch := &parser.ParsedObject{
		ID:   "POL-DEFAULT-300",
		Kind: "completely_unknown_entity_kind_xyz",
		Properties: map[string]any{
			objects.FieldKeyCreatedBy: "ACC-300",
		},
	}
	issues := CheckRegistration(objMismatch, "policy")
	require.NotEmpty(t, issues)
	var foundMismatch bool
	for _, iss := range issues {
		if iss.Tier == 2 && iss.Category == "registration" {
			foundMismatch = true
		}
	}
	assert.True(t, foundMismatch, "expected Tier 2 kind mismatch issue")
}

func TestCheckRegistration_IDValidationFailure(t *testing.T) {
	objBadID := &parser.ParsedObject{
		ID:   "NON_MATCHING_ID_999",
		Kind: "policy",
		Properties: map[string]any{
			objects.FieldKeyCreatedBy: "ACC-400",
		},
	}
	issues := CheckRegistration(objBadID, "policy")
	require.NotEmpty(t, issues)
	var foundIDIssue bool
	for _, iss := range issues {
		if iss.Tier == 2 && iss.Category == "registration" {
			foundIDIssue = true
		}
	}
	assert.True(t, foundIDIssue, "expected Tier 2 ID format issue")
}

func TestHasNonEmptyStringProperty_Variations(t *testing.T) {
	assert.False(t, hasNonEmptyStringProperty(nil, "key"))

	emptyObj := &parser.ParsedObject{Properties: map[string]any{}}
	assert.False(t, hasNonEmptyStringProperty(emptyObj, "missing"))

	blankStrObj := &parser.ParsedObject{Properties: map[string]any{"prop": ""}}
	assert.False(t, hasNonEmptyStringProperty(blankStrObj, "prop"))

	numObj := &parser.ParsedObject{Properties: map[string]any{"prop": 42}}
	assert.False(t, hasNonEmptyStringProperty(numObj, "prop"))

	validObj := &parser.ParsedObject{Properties: map[string]any{"prop": "value"}}
	assert.True(t, hasNonEmptyStringProperty(validObj, "prop"))
}

func TestLooksHandCASMaterialized_Combinations(t *testing.T) {
	assert.True(t, LooksHandCASMaterialized(nil))

	emptyObj := &parser.ParsedObject{Properties: map[string]any{}}
	assert.True(t, LooksHandCASMaterialized(emptyObj))

	onlyCreated := &parser.ParsedObject{Properties: map[string]any{
		objects.FieldKeyCreatedBy: "ACC-1",
	}}
	assert.False(t, LooksHandCASMaterialized(onlyCreated))

	onlyUpdated := &parser.ParsedObject{Properties: map[string]any{
		objects.FieldKeyUpdatedBy: "ACC-2",
	}}
	assert.False(t, LooksHandCASMaterialized(onlyUpdated))

	both := &parser.ParsedObject{Properties: map[string]any{
		objects.FieldKeyCreatedBy: "ACC-1",
		objects.FieldKeyUpdatedBy: "ACC-2",
	}}
	assert.False(t, LooksHandCASMaterialized(both))
}

func TestExtractObjectStatus_AndWithExtractedStatus(t *testing.T) {
	objMissing := &parser.ParsedObject{Properties: map[string]any{}}
	status, issues := extractObjectStatus(objMissing)
	assert.Empty(t, status)
	require.Len(t, issues, 1)
	assert.Equal(t, 2, issues[0].Tier)

	objWithStatus := &parser.ParsedObject{Properties: map[string]any{
		objects.FieldKeyStatus: "in_progress",
	}}
	status, issues = extractObjectStatus(objWithStatus)
	assert.Equal(t, "in_progress", status)
	assert.Empty(t, issues)

	called := false
	callbackIssues := withExtractedStatus(objWithStatus, func(st string) []Issue {
		called = true
		assert.Equal(t, "in_progress", st)
		return []Issue{{Tier: 3, Message: "custom_pass"}}
	})
	assert.True(t, called)
	require.Len(t, callbackIssues, 1)
}

func TestCheckLifecycle_MockValidatorRules(t *testing.T) {
	reg := validation.GetGlobalRegistry()
	origValidator := reg.Get("go")
	t.Cleanup(func() {
		reg.Register("go", origValidator)
		reg.Register("default", origValidator)
	})

	mockVal := &mockLifecycleValidator{
		result: &validation.ValidationResult{
			Errors: []validation.ValidationError{
				{
					Rule:    objects.KindLifecycle,
					Field:   "status",
					Message: "invalid state transition",
				},
				{
					Rule:    "schema",
					Field:   "title",
					Message: "title is required",
				},
			},
		},
	}
	reg.Register("go", mockVal)
	reg.Register("default", mockVal)

	obj := &parser.ParsedObject{
		ID:   "POL-DEFAULT-500",
		Kind: "policy",
		Properties: map[string]any{
			objects.FieldKeyStatus: "draft",
		},
	}

	issues := CheckLifecycle(obj, "policy")
	require.Len(t, issues, 1)
	assert.Equal(t, 1, issues[0].Tier)
	assert.Equal(t, objects.KindLifecycle, issues[0].Category)
	assert.Contains(t, issues[0].Message, "invalid state transition")
}

func TestCheckLifecycle_ValidatorErrorsAndNil(t *testing.T) {
	reg := validation.GetGlobalRegistry()
	origValidator := reg.Get("go")
	t.Cleanup(func() {
		reg.Register("go", origValidator)
		reg.Register("default", origValidator)
	})

	mockValErr := &mockLifecycleValidator{
		err: fmt.Errorf("unexpected validation engine error"),
	}
	reg.Register("go", mockValErr)
	reg.Register("default", mockValErr)

	obj := &parser.ParsedObject{
		ID:   "POL-DEFAULT-501",
		Kind: "policy",
		Properties: map[string]any{
			objects.FieldKeyStatus: "draft",
		},
	}
	issues := CheckLifecycle(obj, "policy")
	assert.Empty(t, issues)

	reg.Register("go", nil)
	reg.Register("default", nil)
	issuesNil := CheckLifecycle(obj, "policy")
	assert.Empty(t, issuesNil)
}

func TestCheckLifecycleWithLoader_EmptyID(t *testing.T) {
	loader := objects.NewLifecycleLoader("")
	obj := &parser.ParsedObject{
		ID:   "",
		Kind: "backlog_item",
		Properties: map[string]any{
			objects.FieldKeyStatus: "completely_invalid_status_xyz",
		},
	}

	issues := CheckLifecycleWithLoader(obj, "backlog_item", loader)
	require.Len(t, issues, 1)
	assert.False(t, issues[0].AutoFixable)
	assert.Empty(t, issues[0].FixCommand)
}

func TestCheckLifecycleWithLoader_FallbackStatus(t *testing.T) {
	root := t.TempDir()

	baseContent := `kind: base_object
statuses:
  - value: base_draft
    description: Base draft state
`
	err := fileutil.WriteFile(filepath.Join(root, "base_object_lifecycle.yaml"), []byte(baseContent), paths.FilePerm644)
	require.NoError(t, err)

	lcContent := `kind: sample_fallback
statuses:
  - value: first_status
    description: First status without origin flag
  - value: second_status
    description: Second status
`
	err = fileutil.WriteFile(filepath.Join(root, "sample_fallback_lifecycle.yaml"), []byte(lcContent), paths.FilePerm644)
	require.NoError(t, err)

	loader := objects.NewLifecycleLoader(root)
	obj := &parser.ParsedObject{
		ID:   "SMP-100",
		Kind: "sample_fallback",
		Properties: map[string]any{
			objects.FieldKeyStatus: "invalid_status",
		},
	}

	issues := CheckLifecycleWithLoader(obj, "sample_fallback", loader)
	require.Len(t, issues, 1)
	assert.True(t, issues[0].AutoFixable)
	assert.Contains(t, issues[0].FixCommand, "--field status=first_status")
}

func TestCheckLifecycleWithLoader_LoadError(t *testing.T) {
	root := t.TempDir()
	loader := objects.NewLifecycleLoader(root)
	obj := &parser.ParsedObject{
		ID:   "SMP-200",
		Kind: "unknown_nonexistent_kind",
		Properties: map[string]any{
			objects.FieldKeyStatus: "invalid_status",
		},
	}

	issues := CheckLifecycleWithLoader(obj, "unknown_nonexistent_kind", loader)
	assert.Empty(t, issues)
}

func TestGetAsyncValidator_WorkerCountVariations(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	vNegative := GetAsyncValidator(ctx, tmpDir, -1)
	require.NotNil(t, vNegative)
	assert.Greater(t, vNegative.GetMaxWorkers(), 0)

	vExplicit := GetAsyncValidator(ctx, tmpDir, 8)
	require.NotNil(t, vExplicit)
	assert.Equal(t, 8, vExplicit.GetMaxWorkers())
}

func TestGetRecommendedSemaphoreCapacity_ContentionThresholds(t *testing.T) {
	metrics := objects.GetGlobalSpecLoaderMetrics()
	metrics.Reset()
	t.Cleanup(func() {
		metrics.Reset()
	})

	capZeroWaits := GetRecommendedSemaphoreCapacity()
	assert.Equal(t, 16, capZeroWaits)

	metrics.RecordLockWait(100 * time.Millisecond)
	capHighContention := GetRecommendedSemaphoreCapacity()
	assert.Equal(t, 32, capHighContention)

	metrics.Reset()
	metrics.RecordLockWait(100 * time.Millisecond)
	for i := 0; i < 4; i++ {
		metrics.RecordLockHold(10 * time.Millisecond)
	}
	capMediumContention := GetRecommendedSemaphoreCapacity()
	assert.Equal(t, 24, capMediumContention)

	metrics.Reset()
	metrics.RecordLockWait(10 * time.Millisecond)
	capLowContention := GetRecommendedSemaphoreCapacity()
	assert.Equal(t, 16, capLowContention)
}

func TestDetermineValidationPriority_NilValidator(t *testing.T) {
	prioRole := DetermineValidationPriority(objects.KindRole, "ROL-1", nil)
	assert.Equal(t, 2, prioRole)

	prioPolicy := DetermineValidationPriority(objects.KindPolicy, "POL-1", nil)
	assert.Equal(t, 2, prioPolicy)

	prioAccount := DetermineValidationPriority(objects.KindAccount, "ACC-1", nil)
	assert.Equal(t, 2, prioAccount)

	prioJob := DetermineValidationPriority(objects.KindSchedulerJob, "JOB-1", nil)
	assert.Equal(t, 2, prioJob)

	prioDefault := DetermineValidationPriority(objects.KindDocEntry, "DOC-1", nil)
	assert.Equal(t, 3, prioDefault)
}

func TestDetermineValidationPriority_CachedIssuesTiers(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	v := GetAsyncValidator(ctx, tmpDir, 2)

	testFilePath := filepath.Join(tmpDir, "test_obj.yaml")
	err := fileutil.WriteFile(testFilePath, []byte("kind: test\n"), paths.FilePerm644)
	require.NoError(t, err)

	now := time.Now()

	// Tier 1 unresolved
	v.SetValidationFunc(func(_ context.Context, objID, kind, path string, _ []byte) (*validation.ValidationState, error) {
		return &validation.ValidationState{
			ObjectID:      objID,
			ObjectKind:    kind,
			FilePath:      path,
			LastValidated: now,
			Issues: []validation.ValidationIssue{
				{Tier: 1, Message: "tier 1 blocking failure"},
			},
		}, nil
	})
	_, err = v.ValidateNow(ctx, "OBJ-TIER-1", "test", testFilePath)
	require.NoError(t, err)
	assert.Equal(t, 1, DetermineValidationPriority("test", "OBJ-TIER-1", v))

	// Tier 2 unresolved
	v.SetValidationFunc(func(_ context.Context, objID, kind, path string, _ []byte) (*validation.ValidationState, error) {
		return &validation.ValidationState{
			ObjectID:      objID,
			ObjectKind:    kind,
			FilePath:      path,
			LastValidated: now,
			Issues: []validation.ValidationIssue{
				{Tier: 2, Message: "tier 2 warning"},
			},
		}, nil
	})
	_, err = v.ValidateNow(ctx, "OBJ-TIER-2", "test", testFilePath)
	require.NoError(t, err)
	assert.Equal(t, 2, DetermineValidationPriority("test", "OBJ-TIER-2", v))

	// Tier 1 resolved
	v.SetValidationFunc(func(_ context.Context, objID, kind, path string, _ []byte) (*validation.ValidationState, error) {
		return &validation.ValidationState{
			ObjectID:      objID,
			ObjectKind:    kind,
			FilePath:      path,
			LastValidated: now,
			Issues: []validation.ValidationIssue{
				{Tier: 1, Message: "tier 1 resolved", ResolvedAt: &now},
			},
		}, nil
	})
	_, err = v.ValidateNow(ctx, "OBJ-RESOLVED-1", objects.KindDocEntry, testFilePath)
	require.NoError(t, err)
	assert.Equal(t, 3, DetermineValidationPriority(objects.KindDocEntry, "OBJ-RESOLVED-1", v))
}

func TestViolationResolver_AllCategoriesAndSuggestions(t *testing.T) {
	root := setupTestSpecDir(t)
	loader := objects.NewSpecLoader(filepath.Join(root, paths.ProcessInternalObjectSpecsDir))
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewTextFormatter(pkgctx.NewSystemContext()))
	resolver := NewViolationResolver(root, loader, logger)

	result := &CheckResult{
		ObjectID:   "SMP-1",
		ObjectKind: "sample",
		FilePath:   "sample.yaml",
	}

	resLifecycle, err := resolver.ResolveViolation(result, Issue{
		Category: objects.KindLifecycle,
		Message:  "Invalid lifecycle status for kind sample",
	})
	require.NoError(t, err)
	assert.True(t, resLifecycle.AutoLinkable)
	assert.Equal(t, 0.8, resLifecycle.Confidence)

	resIntegrity, err := resolver.ResolveViolation(result, Issue{
		Category: "integrity",
		Message:  "Hash mismatch on content verification",
	})
	require.NoError(t, err)
	assert.True(t, resIntegrity.AutoLinkable)
	assert.Equal(t, 0.9, resIntegrity.Confidence)

	resRef, err := resolver.ResolveViolation(result, Issue{
		Category: "reference",
		Message:  "target points to non-existent node",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resRef.Suggestions)
}

func TestViolationResolver_FilterResolvableViolations_Variations(t *testing.T) {
	root := setupTestSpecDir(t)
	loader := objects.NewSpecLoader(filepath.Join(root, paths.ProcessInternalObjectSpecsDir))
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewTextFormatter(pkgctx.NewSystemContext()))
	resolver := NewViolationResolver(root, loader, logger)

	snapshot := &CheckSnapshot{
		Metadata: CheckSnapshotMetadata{
			TotalObjects: 3,
			TotalIssues:  3,
		},
		Results: []CheckResult{
			{
				ObjectID:   "SMP-ALL-RESOLVABLE",
				ObjectKind: "sample",
				Issues: []Issue{
					{Category: "integrity", Message: "Hash mismatch detected"},
				},
			},
			{
				ObjectID:   "SMP-NON-RESOLVABLE",
				ObjectKind: "sample",
				Issues: []Issue{
					{Category: "custom_unknown", Message: "cannot resolve this unknown error"},
				},
			},
			{
				ObjectID:   "SMP-EMPTY-ISSUES",
				ObjectKind: "sample",
				Issues:     []Issue{},
			},
		},
	}

	filtered, err := resolver.FilterResolvableViolations(snapshot)
	require.NoError(t, err)
	require.Len(t, filtered.Results, 1)
	assert.Equal(t, "SMP-NON-RESOLVABLE", filtered.Results[0].ObjectID)
	assert.Equal(t, 1, filtered.Metadata.TotalObjects)
	assert.Equal(t, 1, filtered.Metadata.TotalIssues)
}

func TestCollectTPMFeedbackTelemetry_NilProvider(t *testing.T) {
	ctx := context.Background()
	telemetry, err := CollectTPMFeedbackTelemetry(ctx, nil)
	assert.Error(t, err)
	assert.Nil(t, telemetry)
	assert.Contains(t, err.Error(), "storage provider is required")
}

func TestCollectTPMFeedbackTelemetry_StorageErrors(t *testing.T) {
	ctx := context.Background()

	mockErrCrit := &mockStorageWithError{failKind: objects.KindCriteria}
	telCrit, errCrit := CollectTPMFeedbackTelemetry(ctx, mockErrCrit)
	require.NoError(t, errCrit)
	require.NotNil(t, telCrit)
	assert.Equal(t, 0, telCrit.UnverifiedCriteria)

	mockErrBli := &mockStorageWithError{failKind: objects.KindBacklogItem}
	telBli, errBli := CollectTPMFeedbackTelemetry(ctx, mockErrBli)
	require.NoError(t, errBli)
	require.NotNil(t, telBli)
	assert.Equal(t, 0, telBli.StalledBacklogItems)
}

func TestCollectTPMFeedbackTelemetry_ZeroAnomalies(t *testing.T) {
	ctx := context.Background()
	mockClean := &mockStorageForTelemetry{
		objectsByKind: map[string][]map[string]any{
			objects.KindCriteria: {
				{
					objects.FieldKeyID:     "CRIT-1",
					objects.FieldKeyStatus: objects.ObjectStatusComplete,
				},
				{
					objects.FieldKeyID:     "CRIT-2",
					objects.FieldKeyStatus: objects.ObjectStatusArchived,
				},
			},
			objects.KindBacklogItem: {
				{
					objects.FieldKeyID:          "BLI-1",
					objects.FieldKeyStatus:      objects.ObjectStatusComplete,
					objects.FieldKeyDescription: "Well formed item description",
				},
			},
		},
	}

	tel, err := CollectTPMFeedbackTelemetry(ctx, mockClean)
	require.NoError(t, err)
	require.NotNil(t, tel)
	assert.Equal(t, 0, tel.UnverifiedCriteria)
	assert.Equal(t, 0, tel.StalledBacklogItems)
	assert.Equal(t, 0, tel.HygieneAnomalies)
	assert.Empty(t, tel.RefinementActions)
	assert.Empty(t, tel.RankedActionClusters)
}

func TestSystemCheck_ConcurrentValidationAndResolution(t *testing.T) {
	root := setupTestSpecDir(t)
	loader := objects.NewSpecLoader(filepath.Join(root, paths.ProcessInternalObjectSpecsDir))
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewTextFormatter(pkgctx.NewSystemContext()))
	resolver := NewViolationResolver(root, loader, logger)

	var wg sync.WaitGroup
	concurrencyCount := 10

	for i := 0; i < concurrencyCount; i++ {
		wg.Add(1)
		idx := i
		goroutinelabels.NewGoroutine("test_concurrent_resolution", "resolve violation concurrent execution").StartSimple(func() {
			defer wg.Done()
			res := &CheckResult{
				ObjectID:   fmt.Sprintf("SMP-%d", idx),
				ObjectKind: "sample",
			}
			iss := Issue{Category: "integrity", Message: "Hash mismatch detected"}
			resolved, err := resolver.ResolveViolation(res, iss)
			if err != nil {
				return
			}
			if !resolved.AutoLinkable || resolved.Confidence < 0.8 {
				return
			}
		})
	}

	wg.Wait()
}

func TestSystemCheck_TypesJSONRoundtrip(t *testing.T) {
	iss := Issue{
		Tier:        1,
		Category:    "policy",
		Message:     "blocking test policy violation",
		AutoFixable: true,
		FixCommand:  "zqk object update POL-1",
		IssueClass:  "process_failure",
	}

	issBytes, err := json.Marshal(iss)
	require.NoError(t, err)

	var decodedIss Issue
	err = json.Unmarshal(issBytes, &decodedIss)
	require.NoError(t, err)
	assert.Equal(t, iss, decodedIss)

	snap := CheckSnapshot{
		Metadata: CheckSnapshotMetadata{
			Timestamp:     time.Now().UTC().Truncate(time.Second),
			ProjectRoot:   "/tmp/test",
			TotalObjects:  1,
			TotalIssues:   1,
			BlockingCount: 1,
			WarningCount:  0,
			InfoCount:     0,
			RecCount:      0,
			Command:       "zqk system check",
		},
		Results: []CheckResult{
			{
				ObjectID:   "POL-1",
				ObjectKind: "policy",
				FilePath:   "policies/pol1.yaml",
				Status:     "draft",
				Issues:     []Issue{iss},
				AutoFixed:  []string{"format"},
			},
		},
	}

	snapBytes, err := json.Marshal(snap)
	require.NoError(t, err)

	var decodedSnap CheckSnapshot
	err = json.Unmarshal(snapBytes, &decodedSnap)
	require.NoError(t, err)
	assert.Equal(t, snap.Metadata.TotalObjects, decodedSnap.Metadata.TotalObjects)
	assert.Equal(t, snap.Results[0].ObjectID, decodedSnap.Results[0].ObjectID)
}
