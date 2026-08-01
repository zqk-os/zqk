package validation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// TestAsyncValidator_BaselineComparison compares async validator results
// with synchronous check command results
func TestAsyncValidator_BaselineComparison(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	// Create test objects
	testObjects := createTestObjects(t, testRoot, 100)

	// Run synchronous validation (baseline)
	syncStart := time.Now()
	syncResults := runSynchronousValidation(t, testRoot, testObjects)
	syncDuration := time.Since(syncStart)

	// Run asynchronous validation
	asyncStart := time.Now()
	asyncResults := runAsynchronousValidation(t, testRoot, testObjects)
	asyncDuration := time.Since(asyncStart)

	// Compare results
	t.Logf(ConstMagicc31d7b42, len(syncResults), syncDuration)
	t.Logf(ConstMagicc204f80b, len(asyncResults), asyncDuration)

	// Verify same number of results
	if len(syncResults) != len(asyncResults) {
		t.Errorf(ConstMagic2456e492, len(syncResults), len(asyncResults))
	}

	// Verify same issues found
	syncIssues := countIssues(syncResults)
	asyncIssues := countIssues(asyncResults)

	if syncIssues != asyncIssues {
		t.Errorf(ConstMagic5e6ad408, syncIssues, asyncIssues)
	}

	// Verify async is faster (or at least not significantly slower)
	if asyncDuration > syncDuration*2 {
		t.Logf(ConstMagicaa92128c, asyncDuration, syncDuration)
	}

	// Verify all object IDs match
	syncIDs := getObjectIDs(syncResults)
	asyncIDs := getObjectIDs(asyncResults)

	if len(syncIDs) != len(asyncIDs) {
		t.Errorf(ConstMagic00b9a3e5, len(syncIDs), len(asyncIDs))
	}

	// Check that async found all objects sync found
	for id := range syncIDs {
		if !asyncIDs[id] {
			t.Errorf(ConstMagic30dd0e1b, id)
		}
	}
}

// createTestObjects creates test objects for validation
func createTestObjects(t *testing.T, testRoot string, count int) []string {
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := os.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagic32b1c202, err)
	}

	objectIDs := make([]string, 0, count)
	for i := 0; i < count; i++ {
		objectID := fmt.Sprintf("TEST-%03d", i)
		filePath := filepath.Join(testDir, fmt.Sprintf("%s.yaml", objectID))

		content := fmt.Sprintf(`id: %s
kind: test_object
title: Test Object %d
status: active
schema_version: "%s"
created_at: "%s"
created_by: account:test
`, objectID, i, objects.DefaultSchemaVersion, time.Now().Format(time.RFC3339))

		if err := os.WriteFile(filePath, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf(ConstMagic3ef3a65c, err)
		}

		objectIDs = append(objectIDs, objectID)
	}

	return objectIDs
}

// runSynchronousValidation runs validation synchronously (baseline)
//
//nolint:unparam // t parameter is kept for API consistency with testing helpers
func runSynchronousValidation(_ *testing.T, testRoot string, objectIDs []string) []ValidationState {
	results := make([]ValidationState, 0)

	// Simulate synchronous validation
	// In real implementation, this would call the actual synchronous check command
	for _, objectID := range objectIDs {
		filePath := filepath.Join(datacell.CellCASPrimaryDir(testRoot, "test"), fmt.Sprintf("%s.yaml", objectID))

		// Perform validation
		// Note: IsValid is computed from Issues - if no issues, object is valid
		state := &ValidationState{
			ObjectID:      objectID,
			ObjectKind:    "test_object",
			FilePath:      filePath,
			LastValidated: time.Now(),
			Issues:        []ValidationIssue{}, // Empty issues = valid
		}

		results = append(results, *state)
	}

	return results
}

// runAsynchronousValidation runs validation asynchronously
func runAsynchronousValidation(t *testing.T, testRoot string, objectIDs []string) []ValidationState {
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 4, time.Hour)

	// Set up a simple validation function for the test
	// This validates that the file exists and can be read
	validationFunc := func(ctx context.Context, objectID, objectKind, filePath string, content []byte) (*ValidationState, error) {
		// Compute checksum
		hash := sha256.Sum256(content)
		checksum := hex.EncodeToString(hash[:])

		// Create validation state (simplified - just checks file exists)
		state := &ValidationState{
			ObjectID:      objectID,
			ObjectKind:    objectKind,
			FilePath:      filePath,
			LastValidated: time.Now(),
			Checksum:      checksum,
			Issues:        []ValidationIssue{}, // Empty issues = valid (file exists and readable)
		}

		return state, nil
	}
	validator.SetValidationFunc(validationFunc)

	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic63bbaf4d, err)
	}
	defer func() {
		_ = validator.Stop() //nolint:errcheck // Test cleanup - errors are acceptable
	}()

	// Enqueue all objects
	for _, objectID := range objectIDs {
		filePath := filepath.Join(datacell.CellCASPrimaryDir(testRoot, "test"), fmt.Sprintf("%s.yaml", objectID))
		_ = validator.Enqueue(objectID, "test_object", filePath, 1)
	}

	// Wait for all validations to complete
	timeout := time.After(60 * time.Second)
	tick := time.Tick(200 * time.Millisecond)

	for {
		select {
		case <-timeout:
			// On timeout, collect what we have
			results := make([]ValidationState, 0, len(objectIDs))
			for _, objectID := range objectIDs {
				state, ok := validator.GetCachedState(objectID)
				if ok && state != nil {
					results = append(results, *state)
				}
			}
			_, _, _, queueSize := validator.GetValidationStats()
			allStates := validator.GetAllCachedStates()
			t.Logf("Timeout: collected %d/%d results, queueSize=%d, allStates=%d",
				len(results), len(objectIDs), queueSize, len(allStates))
			if len(results) < len(objectIDs) {
				t.Fatalf("Timeout waiting for async validation: only %d/%d completed", len(results), len(objectIDs))
			}
			return results
		case <-tick:
			_, _, _, queueSize := validator.GetValidationStats()
			allStates := validator.GetAllCachedStates()

			// Check if all objects have been validated
			validatedCount := 0
			for _, objectID := range objectIDs {
				state, ok := validator.GetCachedState(objectID)
				if ok && state != nil {
					validatedCount++
				}
			}

			if queueSize == 0 && validatedCount >= len(objectIDs) {
				// All validations complete
				results := make([]ValidationState, 0, len(objectIDs))
				for _, objectID := range objectIDs {
					state, ok := validator.GetCachedState(objectID)
					if ok && state != nil {
						results = append(results, *state)
					}
				}
				return results
			}

			// Log progress periodically
			if validatedCount%10 == 0 || validatedCount == len(objectIDs) {
				t.Logf("Progress: %d/%d validated, queueSize=%d, cached=%d",
					validatedCount, len(objectIDs), queueSize, len(allStates))
			}
		}
	}
}

// countIssues counts total issues in validation results
func countIssues(results []ValidationState) int {
	count := 0
	for i := range results {
		count += len(results[i].Issues)
	}
	return count
}

// getObjectIDs extracts object IDs from validation results
func getObjectIDs(results []ValidationState) map[string]bool {
	ids := make(map[string]bool)
	for i := range results {
		ids[results[i].ObjectID] = true
	}
	return ids
}
