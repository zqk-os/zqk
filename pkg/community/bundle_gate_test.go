package community

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/bundle"
)

// Helper to construct a valid base manifest for test isolation
func newValidBundleManifest(name string, lang string) *bundle.LanguageBundleManifest {
	return &bundle.LanguageBundleManifest{
		SchemaVersion: "1.0.0",
		Name:          name,
		Language:      lang,
		Version:       "1.0.0",
		Description:   "Modular language bundle for test verification",
		Standards: []bundle.StandardSpec{
			{
				ID:          "STD-001",
				Title:       "Hygiene Standard",
				Description: "Zero panic and clean errors",
				Severity:    "high",
			},
		},
		Gates: []bundle.GateSpec{
			{
				Name:      "unit-tests",
				Command:   "go test ./...",
				EnforceCI: true,
			},
		},
	}
}

// TestBundleGate_StaticFloorInvariant verifies basic constructor integrity, nil rejections, and schema verification.
func TestBundleGate_StaticFloorInvariant(t *testing.T) {
	evaluator := NewProcessPackGateEvaluator("/tmp/test-workspace")
	require.NotNil(t, evaluator, "evaluator constructor must succeed")
	require.Equal(t, "/tmp/test-workspace", evaluator.WorkspaceRoot)
	require.True(t, evaluator.EnforceCIGates)
	require.Equal(t, 16, evaluator.MaxDependencies)

	// Invariant: Evaluating nil manifest must return error
	_, err := evaluator.EvaluateBundle(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot evaluate nil manifest")

	// Invariant: Evaluating nil slice must fail
	_, errSlice := evaluator.EvaluateManifests(nil)
	require.Error(t, errSlice)
	require.Contains(t, errSlice.Error(), "cannot evaluate nil manifests slice")
}

// TestBundleGate_OperationalDynamicProof verifies end-to-end evaluation of valid manifests and aggregation.
func TestBundleGate_OperationalDynamicProof(t *testing.T) {
	tmpDir := t.TempDir()
	evaluator := NewProcessPackGateEvaluator(tmpDir)

	manifestA := newValidBundleManifest("go-toolchain", "go")
	manifestB := newValidBundleManifest("python-toolchain", "python")

	evalA, errA := evaluator.EvaluateBundle(manifestA)
	require.NoError(t, errA)
	require.Equal(t, PackGatePass, evalA.Status)
	require.Empty(t, evalA.Findings)

	evalB, errB := evaluator.EvaluateBundle(manifestB)
	require.NoError(t, errB)
	require.Equal(t, PackGatePass, evalB.Status)

	manifests := []*bundle.LanguageBundleManifest{manifestA, manifestB}
	report, errReport := evaluator.EvaluateManifests(manifests)
	require.NoError(t, errReport)
	require.Equal(t, 2, report.TotalPacks.Total)
	require.Equal(t, 2, report.Passed)
	require.Equal(t, 0, report.Failed)
}

// TestBundleGate_NegativeBoundaryAndErrorRecovery tests path traversal, missing gate commands, and dependency caps.
func TestBundleGate_NegativeBoundaryAndErrorRecovery(t *testing.T) {
	evaluator := NewProcessPackGateEvaluator("/workspace")

	// 1. Dependency limit violation
	manifestWithExcessDeps := newValidBundleManifest("excessive-deps", "rust")
	manifestWithExcessDeps.Dependencies = []string{
		"d1", "d2", "d3", "d4", "d5", "d6", "d7", "d8",
		"d9", "d10", "d11", "d12", "d13", "d14", "d15", "d16", "d17",
	}
	resLimit, errLimit := evaluator.EvaluateBundle(manifestWithExcessDeps)
	require.NoError(t, errLimit)
	require.Equal(t, PackGateFail, resLimit.Status)
	require.NotEmpty(t, resLimit.Findings)
	require.Equal(t, "BUNDLE-LIMIT-002", resLimit.Findings[0].RuleID)

	// 2. Directive path escape attempt
	manifestWithEscape := newValidBundleManifest("escaped-directive", "bash")
	manifestWithEscape.AgentDirectives = []bundle.DirectiveSpec{
		{
			TargetRole: "craftsman",
			Directive:  "Follow instructions",
			Path:       "../../etc/passwd",
		},
	}
	resEscape, errEscape := evaluator.EvaluateBundle(manifestWithEscape)
	require.NoError(t, errEscape)
	require.Equal(t, PackGateFail, resEscape.Status)
	require.NotEmpty(t, resEscape.Findings)

	// 3. Enforced CI gate with empty command
	manifestWithEmptyGate := newValidBundleManifest("broken-gate", "ruby")
	manifestWithEmptyGate.Gates = []bundle.GateSpec{
		{
			Name:      "test-run",
			Command:   "   ",
			EnforceCI: true,
		},
	}
	resGate, errGate := evaluator.EvaluateBundle(manifestWithEmptyGate)
	require.NoError(t, errGate)
	require.Equal(t, PackGateFail, resGate.Status)
	require.Contains(t, resGate.Findings[0].Message, "missing command")

	// 4. Aggregated failure reporting
	mixedList := []*bundle.LanguageBundleManifest{
		manifestWithExcessDeps,
		manifestWithEmptyGate,
	}
	report, errMixed := evaluator.EvaluateManifests(mixedList)
	require.NoError(t, errMixed)
	require.Equal(t, 2, report.TotalPacks.Total)
	require.Equal(t, 0, report.Passed)
	require.Equal(t, 2, report.Failed)
}

// TestBundleGate_DirectoryCleanPath verifies path resolution cleanliness and normalization.
func TestBundleGate_DirectoryCleanPath(t *testing.T) {
	eval := NewProcessPackGateEvaluator("/tmp/nested/../test/./")
	expected := filepath.Clean("/tmp/test")
	require.Equal(t, expected, eval.WorkspaceRoot)
}
