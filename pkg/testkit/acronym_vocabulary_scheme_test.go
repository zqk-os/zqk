package testkit_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/acronyms"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestAcronymVocabulary_StaticFloor verifies CRIT-1790814939731525000-2151219e.
// Asserts that VOC-KERNEL-ACRONYMS exists in the Knowledge Kernel and the catalog has >=16 entries.
func TestAcronymVocabulary_StaticFloor(t *testing.T) {
	t.Parallel()

	// 1. Catalog static floor
	all := acronyms.ListAll()
	assert.GreaterOrEqual(t, len(all), 16, "acronym catalog must contain at least 16 core acronyms")

	// 2. Storage scheme verification via direct safe file read (bypasses test WAL repo guard)
	projectRoot := paths.ResolveProjectRoot(".")
	matches, err := filepath.Glob(filepath.Join(projectRoot, ".zqk", "*", "vocabulary_scheme", "*", "VOC-KERNEL-ACRONYMS.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, matches, "VOC-KERNEL-ACRONYMS.yaml must exist on disk in the Knowledge Kernel")

	data, err := fileutil.ReadFile(matches[0])
	require.NoError(t, err)
	var obj map[string]any
	require.NoError(t, yaml.Unmarshal(data, &obj))
	assert.Equal(t, "vocabulary_scheme", obj[objects.FieldKeyKind])
	assert.Equal(t, "operational", obj["context_scope"])
}

// TestAcronymVocabulary_OperationalProof verifies CRIT-1790814939731526000-641cda96.
// Asserts case-insensitive retrieval and full progressive disclosure fields for core acronyms.
func TestAcronymVocabulary_OperationalProof(t *testing.T) {
	t.Parallel()

	// Check BLI
	bli, found := acronyms.Lookup("bli")
	require.True(t, found)
	assert.Equal(t, "BLI", bli.Code)
	assert.Equal(t, "Backlog Item", bli.FullName)
	assert.NotEmpty(t, bli.Definition)
	assert.NotEmpty(t, bli.Context)
	assert.Contains(t, bli.RelatedRefs, "PRI")

	// Check VDS
	vds, found := acronyms.Lookup("VDS")
	require.True(t, found)
	assert.Equal(t, "VDS", vds.Code)
	assert.Contains(t, vds.FullName, "Verification")
	assert.NotEmpty(t, vds.Definition)

	// Check table format
	table := acronyms.FormatTable(acronyms.ListAll())
	assert.Contains(t, table, "BLI")
	assert.Contains(t, table, "VDS")
	assert.Contains(t, table, "PPLAN")
}

// TestAcronymVocabulary_NegativeBoundary verifies CRIT-1790814939731527000-d79aae82.
// Asserts graceful rejection of unknown acronyms with fuzzy suggestions and no panics.
func TestAcronymVocabulary_NegativeBoundary(t *testing.T) {
	t.Parallel()

	// Empty query
	_, found := acronyms.Lookup("")
	assert.False(t, found)

	// Unknown query with suggestion
	_, found = acronyms.Lookup("PLN")
	assert.False(t, found)
	suggestions := acronyms.FindClosest("PLN")
	assert.Contains(t, suggestions, "PPLAN")

	// Completely foreign query
	_, found = acronyms.Lookup("XYZ999FOOBAR")
	assert.False(t, found)
}
