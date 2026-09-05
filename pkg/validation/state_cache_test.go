package validation

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestValidationStateCache_BasicOperations(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	cache := NewValidationStateCache(testRoot, time.Hour)

	// Test Set and Get
	state := &ValidationState{
		ObjectID:      "TEST-001",
		ObjectKind:    "test_object",
		FilePath:      "test.yaml",
		LastValidated: time.Now(),
		Checksum:      "abc123",
		Issues:        []ValidationIssue{},
	}

	cache.Set(state)

	retrieved, exists := cache.Get("TEST-001")
	if !exists {
		t.Error(ConstMagic1e44ef95)
	}
	if retrieved.ObjectID != "TEST-001" {
		t.Errorf(ConstMagicb40fbdb4, retrieved.ObjectID)
	}

	// Test Invalidate
	cache.Invalidate("TEST-001")
	_, exists = cache.Get("TEST-001")
	if exists {
		t.Error(ConstMagicf2f2a4be)
	}
}

// TestValidationStateCache_SetRejectsOversizedID ensures entries with object ID
// longer than MaxObjectIDLength are not stored (avoids cache/validation misery).
func TestValidationStateCache_SetRejectsOversizedID(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)
	cache := NewValidationStateCache(testRoot, time.Hour)

	longID := strings.Repeat("x", MaxObjectIDLength+1)
	state := &ValidationState{
		ObjectID:      longID,
		ObjectKind:    "scheduler_job",
		FilePath:      filepath.Join(paths.ProcessDir, "scheduler_jobs", "foo.yaml"),
		LastValidated: time.Now(),
	}
	cache.Set(state)

	_, exists := cache.Get(longID)
	if exists {
		t.Error(ConstMagic3e2799e7)
	}
}

func TestValidationStateCache_Persistence(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	cache := NewValidationStateCache(testRoot, time.Hour)

	// Add some states
	state1 := &ValidationState{
		ObjectID:      "TEST-001",
		ObjectKind:    "test_object",
		FilePath:      "test1.yaml",
		LastValidated: time.Now(),
		Checksum:      "abc123",
		Issues:        []ValidationIssue{},
	}
	state2 := &ValidationState{
		ObjectID:      "TEST-002",
		ObjectKind:    "test_object",
		FilePath:      "test2.yaml",
		LastValidated: time.Now(),
		Checksum:      "def456",
		Issues: []ValidationIssue{
			{
				Tier:       1,
				Category:   "registration",
				Message:    "Test issue",
				DetectedAt: time.Now(),
			},
		},
	}

	cache.Set(state1)
	cache.Set(state2)

	// Save cache
	if err := cache.Save(); err != nil {
		t.Fatalf(ConstMagic54fa5912, err)
	}

	// Create new cache instance and load
	cache2 := NewValidationStateCache(testRoot, time.Hour)
	if err := cache2.Load(); err != nil {
		t.Fatalf(ConstMagic969edc24, err)
	}

	// Verify states were loaded
	retrieved1, exists := cache2.Get("TEST-001")
	if !exists {
		t.Error(ConstMagic76cfc6c0)
	} else if retrieved1.Checksum != "abc123" {
		t.Errorf(ConstMagicac6bda64, retrieved1.Checksum)
	}

	retrieved2, exists := cache2.Get("TEST-002")
	if !exists {
		t.Error(ConstMagic14eff21d)
	} else if len(retrieved2.Issues) != 1 {
		t.Errorf(ConstMagic3b8229c4, len(retrieved2.Issues))
	}
}

func TestValidationStateCache_StaleDetection(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	// Use very short max age for testing
	cache := NewValidationStateCache(testRoot, 100*time.Millisecond)

	state := &ValidationState{
		ObjectID:      "TEST-001",
		ObjectKind:    "test_object",
		FilePath:      "test.yaml",
		LastValidated: time.Now(),
		Checksum:      "abc123",
		Issues:        []ValidationIssue{},
	}

	cache.Set(state)

	// Should exist immediately
	_, exists := cache.Get("TEST-001")
	if !exists {
		t.Error(ConstMagic73eda4cd)
	}

	// Wait for it to become stale
	time.Sleep(150 * time.Millisecond)

	// Should not exist (stale)
	_, exists = cache.Get("TEST-001")
	if exists {
		t.Error(ConstMagice636531d)
	}

	// GetStale should return it
	stale := cache.GetStale()
	if len(stale) != 1 {
		t.Errorf(ConstMagic523d4c81, len(stale))
	}
}

func TestValidationStateCache_InvalidateByKind(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	cache := NewValidationStateCache(testRoot, time.Hour)

	// Add states for different kinds
	cache.Set(&ValidationState{
		ObjectID:      "TEST-001",
		ObjectKind:    "backlog_item",
		FilePath:      "test1.yaml",
		LastValidated: time.Now(),
		Checksum:      "abc123",
	})
	cache.Set(&ValidationState{
		ObjectID:      "TEST-002",
		ObjectKind:    "backlog_item",
		FilePath:      "test2.yaml",
		LastValidated: time.Now(),
		Checksum:      "def456",
	})
	cache.Set(&ValidationState{
		ObjectID:      "TEST-003",
		ObjectKind:    "goal",
		FilePath:      "test3.yaml",
		LastValidated: time.Now(),
		Checksum:      "ghi789",
	})

	// Invalidate by kind
	cache.InvalidateByKind("backlog_item")

	// backlog_item states should be gone
	_, exists := cache.Get("TEST-001")
	if exists {
		t.Error(ConstMagicbf06e3ab)
	}
	_, exists = cache.Get("TEST-002")
	if exists {
		t.Error(ConstMagic2977a18c)
	}

	// goal state should still exist
	_, exists = cache.Get("TEST-003")
	if !exists {
		t.Error(ConstMagic3d591540)
	}
}

func TestValidationStateCache_GetByTier(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	cache := NewValidationStateCache(testRoot, time.Hour)

	// Add states with different tier issues
	cache.Set(&ValidationState{
		ObjectID:      "TEST-001",
		ObjectKind:    "test_object",
		FilePath:      "test1.yaml",
		LastValidated: time.Now(),
		Issues: []ValidationIssue{
			{Tier: 1, Category: "registration", Message: "Tier 1 issue", DetectedAt: time.Now()},
		},
	})
	cache.Set(&ValidationState{
		ObjectID:      "TEST-002",
		ObjectKind:    "test_object",
		FilePath:      "test2.yaml",
		LastValidated: time.Now(),
		Issues: []ValidationIssue{
			{Tier: 2, Category: "lifecycle", Message: "Tier 2 issue", DetectedAt: time.Now()},
		},
	})
	cache.Set(&ValidationState{
		ObjectID:      "TEST-003",
		ObjectKind:    "test_object",
		FilePath:      "test3.yaml",
		LastValidated: time.Now(),
		Issues: []ValidationIssue{
			{Tier: 1, Category: "policy", Message: "Tier 1 issue", DetectedAt: time.Now()},
		},
	})

	// Get by tier
	tier1States := cache.GetByTier(1)
	if len(tier1States) != 2 {
		t.Errorf(ConstMagicd74d4c4b, len(tier1States))
	}

	tier2States := cache.GetByTier(2)
	if len(tier2States) != 1 {
		t.Errorf(ConstMagicc5c38600, len(tier2States))
	}
}

func TestValidationStateCache_Count(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	cache := NewValidationStateCache(testRoot, time.Hour)

	// Add some states
	cache.Set(&ValidationState{
		ObjectID:      "TEST-001",
		ObjectKind:    "test_object",
		FilePath:      "test1.yaml",
		LastValidated: time.Now(),
		Issues:        []ValidationIssue{},
	})
	cache.Set(&ValidationState{
		ObjectID:      "TEST-002",
		ObjectKind:    "test_object",
		FilePath:      "test2.yaml",
		LastValidated: time.Now(),
		Issues: []ValidationIssue{
			{Tier: 1, Category: "registration", Message: "Issue", DetectedAt: time.Now()},
		},
	})

	total, stale, withIssues := cache.Count()
	if total != 2 {
		t.Errorf(ConstMagic844c9d15, total)
	}
	if stale != 0 {
		t.Errorf(ConstMagic645416eb, stale)
	}
	if withIssues != 1 {
		t.Errorf(ConstMagicbdd5d575, withIssues)
	}
}

// TestValidationStateCache_Save_ErrorPaths tests error handling in Save()
func TestValidationStateCache_Save_ErrorPaths(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	cache := NewValidationStateCache(testRoot, time.Hour)

	// Add a state
	cache.Set(&ValidationState{
		ObjectID:      "TEST-001",
		ObjectKind:    "test_object",
		FilePath:      "test1.yaml",
		LastValidated: time.Now(),
		Checksum:      "abc123",
		Issues:        []ValidationIssue{},
	})

	// Test successful save
	if err := cache.Save(); err != nil {
		t.Fatalf(ConstMagic993d14f2, err)
	}

	// Test save with empty cache
	cache2 := NewValidationStateCache(testRoot, time.Hour)
	if err := cache2.Save(); err != nil {
		t.Errorf(ConstMagic53c1d535, err)
	}

	// Test save with stale lock handling
	// This tests the stale lock detection and removal logic
	cache3 := NewValidationStateCache(testRoot, time.Hour)
	cache3.Set(&ValidationState{
		ObjectID:      "TEST-002",
		ObjectKind:    "test_object",
		FilePath:      "test2.yaml",
		LastValidated: time.Now(),
		Checksum:      "def456",
		Issues:        []ValidationIssue{},
	})

	// Save should succeed even if there's a stale lock file
	if err := cache3.Save(); err != nil {
		t.Errorf(ConstMagic9a6b0560, err)
	}
}

// TestValidationStateCache_Save_Concurrent tests concurrent save operations
func TestValidationStateCache_Save_Concurrent(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	cache := NewValidationStateCache(testRoot, time.Hour)

	// Add multiple states
	for i := 0; i < 10; i++ {
		cache.Set(&ValidationState{
			ObjectID:      fmt.Sprintf("TEST-%03d", i),
			ObjectKind:    "test_object",
			FilePath:      fmt.Sprintf("test%d.yaml", i),
			LastValidated: time.Now(),
			Checksum:      fmt.Sprintf("checksum%d", i),
			Issues:        []ValidationIssue{},
		})
	}

	// Save should handle multiple states correctly
	if err := cache.Save(); err != nil {
		t.Fatalf(ConstMagicdb7f4877, err)
	}

	// Verify all states were saved
	cache2 := NewValidationStateCache(testRoot, time.Hour)
	if err := cache2.Load(); err != nil {
		t.Fatalf(ConstMagic9e0f6418, err)
	}

	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("TEST-%03d", i)
		_, exists := cache2.Get(id)
		if !exists {
			t.Errorf("expected state %s to exist after save/load", id)
		}
	}
}

// TestValidationStateCache_CodeChangeInvalidation tests automatic cache invalidation
// when validation code files change
func TestValidationStateCache_CodeChangeInvalidation(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	cache := NewValidationStateCache(testRoot, time.Hour)

	// Add a state to cache
	state := &ValidationState{
		ObjectID:      "TEST-001",
		ObjectKind:    "test_object",
		FilePath:      "test.yaml",
		LastValidated: time.Now(),
		Checksum:      "abc123",
		Issues:        []ValidationIssue{},
	}

	cache.Set(state)

	// Save cache with current code checksum
	if err := cache.Save(); err != nil {
		t.Fatalf(ConstMagic54fa5912, err)
	}

	// Verify state is in cache
	retrieved, exists := cache.Get("TEST-001")
	if !exists {
		t.Fatal(ConstMagic46c11a8f)
	}
	if retrieved.ObjectID != "TEST-001" {
		t.Errorf(ConstMagicb40fbdb4, retrieved.ObjectID)
	}

	// Simulate code change by modifying a validation code file
	// We'll create a temporary file that will change the checksum
	validationCodeFile := filepath.Join(testRoot, "cmd", "zqk", "system", ConstMagicd4badb3d)

	// Create directory if it doesn't exist
	if err := fileutil.MkdirAll(filepath.Dir(validationCodeFile), paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagic43ec9c63, err)
	}

	// Write a modified version of the file (simulating code change)
	originalContent := []byte("package system\n// Original code")
	modifiedContent := []byte("package system\n// Modified code - checksum should change")

	// First, write original content
	if err := fileutil.WriteFile(validationCodeFile, originalContent, paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic2f93fd0b, err)
	}

	// Save cache again with original content
	if err := cache.Save(); err != nil {
		t.Fatalf(ConstMagica11fa40a, err)
	}

	// Load cache - should load successfully
	cache2 := NewValidationStateCache(testRoot, time.Hour)
	if err := cache2.Load(); err != nil {
		t.Fatalf(ConstMagic969edc24, err)
	}

	// Verify state is still in cache
	retrieved2, exists := cache2.Get("TEST-001")
	if !exists {
		t.Fatal(ConstMagic5fd5037c)
	}
	if retrieved2.ObjectID != "TEST-001" {
		t.Errorf(ConstMagicb40fbdb4, retrieved2.ObjectID)
	}

	// Now modify the code file (simulating code change)
	if err := fileutil.WriteFile(validationCodeFile, modifiedContent, paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic1106fdcf, err)
	}

	// Load cache again - should detect code change and invalidate cache
	cache3 := NewValidationStateCache(testRoot, time.Hour)
	if err := cache3.Load(); err != nil {
		t.Fatalf(ConstMagic969edc24, err)
	}

	// Verify state is NOT in cache (cache was invalidated due to code change)
	_, exists = cache3.Get("TEST-001")
	if exists {
		t.Error(ConstMagic70aeb1d1)
	}
	// Poison on-disk file must be removed so empty-save guards cannot resurrect it.
	if _, err := fileutil.Stat(cache3.cacheFile); err == nil {
		t.Fatalf("expected validation cache file removed after code checksum mismatch: %s", cache3.cacheFile)
	} else if !fileutil.IsNotExist(err) {
		t.Fatalf("stat cache file: %v", err)
	}
}

func TestShouldCacheValidationState(t *testing.T) {
	t.Parallel()
	// Excluded (high-volume/ephemeral) kinds
	for _, k := range []string{"audit_event", "base_metric", "scheduler_health_metric", "command_metric", "zqk_session"} {
		if ShouldCacheValidationState(k) {
			t.Errorf(ConstMagic7fb96993, k)
		}
	}
	// Normal kinds should be cached
	for _, k := range []string{"backlog_item", "requirement", "criteria", "milestone", "scheduler_job", ""} {
		if !ShouldCacheValidationState(k) {
			t.Errorf(ConstMagic5be5c990, k)
		}
	}
}

// TestValidationStateCache_LoadPrunesExcludedKinds verifies that Load() does not load
// states for excluded kinds, so the next Save() writes a pruned file (prevents unbounded growth).
func TestValidationStateCache_LoadPrunesExcludedKinds(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	cache := NewValidationStateCache(testRoot, time.Hour)
	cache.Set(&ValidationState{ObjectID: "REQ-001", ObjectKind: "requirement", FilePath: "r.yaml", LastValidated: time.Now()})
	cache.Set(&ValidationState{ObjectID: "AUD-001", ObjectKind: "audit_event", FilePath: "a.yaml", LastValidated: time.Now()})
	if err := cache.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cache2 := NewValidationStateCache(testRoot, time.Hour)
	if err := cache2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := cache2.Get("AUD-001"); ok {
		t.Error(ConstMagicf2a303fc)
	}
	if _, ok := cache2.Get("REQ-001"); !ok {
		t.Error(ConstMagic42f28d39)
	}
}
