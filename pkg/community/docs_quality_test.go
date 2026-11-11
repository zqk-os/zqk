package community_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/vet"
)

// TestDocsQuality_StaticFloorInvariant satisfies CRIT-1794386722508028000-f4166af5.
// Verifies gates.yaml defines immutable topic character budget limits:
// 4,000 characters for concept/guides and 8,000 characters for architecture/specs.
func TestDocsQuality_StaticFloorInvariant(t *testing.T) {
	repoRoot := findRepoRootForSDK(t)
	gatesPath := filepath.Join(repoRoot, "config", "gates.yaml")

	data, err := fileutil.ReadFile(gatesPath)
	require.NoError(t, err, "config/gates.yaml must exist")
	content := string(data)

	assert.Contains(t, content, "doc_scrutiny:", "gates.yaml must configure doc_scrutiny")
	assert.Contains(t, content, "max_topic_chars_concept: 4000", "concept topic limit must be 4000 chars")
	assert.Contains(t, content, "max_topic_chars_architecture: 8000", "architecture topic limit must be 8000 chars")
	assert.Contains(t, content, "fail_on_topic_overflow: true", "fail_on_topic_overflow must be enabled")
}

// TestDocsQuality_OperationalDynamicProof satisfies CRIT-1794386730030285000-fcead641.
// Runs the documentation quality checker across public docs and verifies zero errors.
func TestDocsQuality_OperationalDynamicProof(t *testing.T) {
	repoRoot := findRepoRootForSDK(t)

	cfg := &vet.GatesConfig{
		DocScrutiny: vet.DocScrutinyConfig{
			Enabled:                   true,
			MaxTopicCharsConcept:      4000,
			MaxTopicCharsArchitecture: 8000,
			FailOnTopicOverflow:       true,
			ProhibitCycleWork:         true,
			ProhibitKernelIDLeaks:     true,
			CheckAcronymConsistency:   true,
			Exemptions: []string{
				"docs/releases/*",
				"docs/archive/*",
			},
		},
	}

	findings, err := vet.CheckDocumentationQuality(repoRoot, cfg)
	require.NoError(t, err, "CheckDocumentationQuality should execute without system error")

	var errorFindings []vet.Finding
	for _, f := range findings {
		if f.Severity == vet.SeverityError {
			errorFindings = append(errorFindings, f)
		}
	}

	assert.Empty(t, errorFindings, "public documentation must have zero blocking quality errors: %+v", errorFindings)
}

// TestDocsQuality_NegativeAdversarialBoundary satisfies CRIT-1794386736393016000-762188b4.
// Verifies fail-closed detection and rejection of raw nanosecond kernel IDs,
// internal sprint narratives, and contradictory acronym expansions.
func TestDocsQuality_NegativeAdversarialBoundary(t *testing.T) {
	tmpDir := t.TempDir()
	docsDir := filepath.Join(tmpDir, "docs", "guides")
	require.NoError(t, fileutil.EnsureDir(docsDir))

	// 1. Synthetic doc with leaked nanosecond kernel ID
	leakedIDDoc := filepath.Join(docsDir, "leaked_id.md")
	err := fileutil.WriteFile(leakedIDDoc, []byte("# Guide\n\n## Overview\nRefer to task BLI-1794386770780862000-f34f11a0 for implementation.\n"), 0644)
	require.NoError(t, err)

	// 2. Synthetic doc with internal cycle sprint narrative
	cycleNarrativeDoc := filepath.Join(docsDir, "cycle_narrative.md")
	err = fileutil.WriteFile(cycleNarrativeDoc, []byte("# Guide\n\n## History\nThis was achieved in Cycle 42 during sprint burndown.\n"), 0644)
	require.NoError(t, err)

	// 3. Synthetic doc with contradictory acronym definition
	badAcronymDoc := filepath.Join(docsDir, "bad_acronym.md")
	err = fileutil.WriteFile(badAcronymDoc, []byte("# Guide\n\n## Components\nUsing ATK as our automated test kit for verification.\n"), 0644)
	require.NoError(t, err)

	cfg := &vet.GatesConfig{
		DocScrutiny: vet.DocScrutinyConfig{
			Enabled:                   true,
			MaxTopicCharsConcept:      4000,
			MaxTopicCharsArchitecture: 8000,
			FailOnTopicOverflow:       true,
			ProhibitCycleWork:         true,
			ProhibitKernelIDLeaks:     true,
			CheckAcronymConsistency:   true,
		},
	}

	findings, err := vet.CheckDocumentationQuality(tmpDir, cfg)
	require.NoError(t, err)

	var foundIDLeak, foundCycleLeak, foundBadAcronym bool
	for _, f := range findings {
		if strings.Contains(f.CheckID, "kernel_id_leak") {
			foundIDLeak = true
		}
		if strings.Contains(f.CheckID, "cycle_narrative_leak") {
			foundCycleLeak = true
		}
		if strings.Contains(f.CheckID, "contradictory_acronym") {
			foundBadAcronym = true
		}
	}

	assert.True(t, foundIDLeak, "must detect and fail-close on leaked nanosecond kernel IDs")
	assert.True(t, foundCycleLeak, "must detect and fail-close on internal cycle sprint narratives")
	assert.True(t, foundBadAcronym, "must detect and fail-close on contradictory acronym expansions")
}
