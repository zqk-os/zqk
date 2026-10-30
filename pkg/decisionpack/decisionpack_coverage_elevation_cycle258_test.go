package decisionpack_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/decisionpack"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestCycle258_DecisionPackConstants validates package level constants.
func TestCycle258_DecisionPackConstants(t *testing.T) {
	assert.Equal(t, "decision", decisionpack.Name)
	assert.Equal(t, "packs/decision/specs", decisionpack.SpecDir)
	assert.Equal(t, "packs/decision/lifecycles", decisionpack.LifecycleDir)
	assert.Equal(t, "zqk_omit_decisionpack", decisionpack.OmitBuildTag)
}

// TestCycle258_DecisionPackKindsEnumeration validates kind enumeration, order, and uniqueness.
func TestCycle258_DecisionPackKindsEnumeration(t *testing.T) {
	kinds := decisionpack.Kinds()
	require.NotEmpty(t, kinds)
	assert.Equal(t, 3, len(kinds))

	expectedKinds := []string{
		"decision",
		"impact_analysis",
		"question",
	}
	assert.Equal(t, expectedKinds, kinds)

	seen := make(map[string]bool)
	for _, k := range kinds {
		assert.NotEmpty(t, k)
		assert.False(t, seen[k], "duplicate kind found: "+k)
		seen[k] = true
	}
}

// TestCycle258_DecisionPackOwnedKindsVerification validates individual kind memberships.
func TestCycle258_DecisionPackOwnedKindsVerification(t *testing.T) {
	kinds := decisionpack.Kinds()
	kindMap := make(map[string]struct{}, len(kinds))
	for _, k := range kinds {
		kindMap[k] = struct{}{}
	}

	// Verify all owned kinds are present
	assert.Contains(t, kindMap, "decision")
	assert.Contains(t, kindMap, "impact_analysis")
	assert.Contains(t, kindMap, "question")

	// Verify unowned kinds are absent
	assert.NotContains(t, kindMap, "goal")
	assert.NotContains(t, kindMap, "milestone")
	assert.NotContains(t, kindMap, "unknown_kind")
}

// TestCycle258_DecisionPackBuildTags validates conditional compilation tag conventions.
func TestCycle258_DecisionPackBuildTags(t *testing.T) {
	assert.Equal(t, "zqk_omit_decisionpack", decisionpack.OmitBuildTag)
	assert.True(t, len(decisionpack.OmitBuildTag) > len("zqk_omit_"))
	assert.Equal(t, "zqk_omit_"+decisionpack.Name+"pack", decisionpack.OmitBuildTag)
}

// TestCycle258_DecisionPackRegistrationIntegration validates integration with the central object registration system.
func TestCycle258_DecisionPackRegistrationIntegration(t *testing.T) {
	// Register the pack kinds with objects central registry
	objects.AddPackOwnedKinds(decisionpack.Kinds())

	registered := objects.PackOwnedKinds()
	for _, kind := range decisionpack.Kinds() {
		assert.Contains(t, registered, kind, "expected kind %s to be present in registered pack kinds", kind)
	}

	// Verify SpecDir and LifecycleDir registration works without panic
	assert.NotPanics(t, func() {
		objects.AddModuleSpecRoot(decisionpack.SpecDir)
		objects.AddModuleLifecycleRoot(decisionpack.LifecycleDir)
	})
}

// TestCycle258_DecisionPackConcurrencyAndRaceSafety verifies concurrent calls to Kinds() exhibit no race conditions.
func TestCycle258_DecisionPackConcurrencyAndRaceSafety(t *testing.T) {
	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		goroutinelabels.NewGoroutine("test_decisionpack_concurrency", "validating thread safety").StartSimple(func() {
			defer wg.Done()
			kinds := decisionpack.Kinds()
			assert.Equal(t, 3, len(kinds))
			assert.Equal(t, "decision", kinds[0])
			assert.Equal(t, "impact_analysis", kinds[1])
			assert.Equal(t, "question", kinds[2])
		})
	}

	wg.Wait()
}

// TestCycle258_DecisionPackSpecAndLifecycleFilesExistence verifies the physical existence of declared specs and lifecycles.
func TestCycle258_DecisionPackSpecAndLifecycleFilesExistence(t *testing.T) {
	// Search upward to find repository root with go.mod
	dir, err := os.Getwd()
	require.NoError(t, err)

	var root string
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			root = dir
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	require.NotEmpty(t, root, "repository root containing go.mod must be found")

	specBase := filepath.Join(root, decisionpack.SpecDir)
	lifecycleBase := filepath.Join(root, decisionpack.LifecycleDir)

	// Verify specs exist
	for _, kind := range decisionpack.Kinds() {
		specPath := filepath.Join(specBase, kind+".yaml")
		_, err := os.Stat(specPath)
		assert.NoError(t, err, "spec file should exist: %s", specPath)
	}

	// Verify lifecycles exist for kinds that declare them
	lifecycledKinds := []string{"decision", "question"}
	for _, kind := range lifecycledKinds {
		lifecyclePath := filepath.Join(lifecycleBase, kind+"_lifecycle.yaml")
		_, err := os.Stat(lifecyclePath)
		assert.NoError(t, err, "lifecycle file should exist: %s", lifecyclePath)
	}
}
