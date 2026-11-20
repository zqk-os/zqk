package community

import (
	"fmt"
	"math"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestSemanticDedup_StaticFloorInvariant(t *testing.T) {
	valid := SemanticEntity{
		ID:           "node-alpha",
		Label:        "Authentication Service",
		ParentBranch: "main",
		Embedding:    []float64{0.8, 0.6, 0.0},
	}
	require.NoError(t, ValidateSemanticEntity(valid))

	entityCases := []struct {
		name   string
		entity SemanticEntity
	}{
		{name: "blank_id", entity: SemanticEntity{ID: "  ", Label: "Valid", Embedding: []float64{1.0}}},
		{name: "blank_label", entity: SemanticEntity{ID: "id-1", Label: "\t\n", Embedding: []float64{1.0}}},
		{name: "empty_embedding", entity: SemanticEntity{ID: "id-1", Label: "Valid", Embedding: nil}},
		{name: "nan_embedding", entity: SemanticEntity{ID: "id-1", Label: "Valid", Embedding: []float64{1.0, math.NaN()}}},
		{name: "inf_embedding", entity: SemanticEntity{ID: "id-1", Label: "Valid", Embedding: []float64{math.Inf(1), 0.5}}},
	}
	for _, tc := range entityCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Error(t, ValidateSemanticEntity(tc.entity))
		})
	}

	validCfg := DedupConfig{SimilarityThreshold: 0.85, CosineWeight: 0.7, LexicalWeight: 0.3}
	require.NoError(t, ValidateDedupConfig(validCfg))
	require.NoError(t, ValidateDedupConfig(DedupConfig{SimilarityThreshold: 1.0}))

	badCfgs := []DedupConfig{
		{SimilarityThreshold: 0.0},
		{SimilarityThreshold: -0.25},
		{SimilarityThreshold: 1.05},
		{SimilarityThreshold: math.NaN()},
		{SimilarityThreshold: math.Inf(1)},
		{SimilarityThreshold: 0.8, CosineWeight: -0.1, LexicalWeight: 0.5},
		{SimilarityThreshold: 0.8, CosineWeight: 0.5, LexicalWeight: math.Inf(-1)},
	}
	for i, cfg := range badCfgs {
		t.Run(fmt.Sprintf("invalid_cfg_%d", i), func(t *testing.T) {
			assert.Error(t, ValidateDedupConfig(cfg))
			_, err := NewSemanticDedupEngine(cfg)
			assert.Error(t, err)
		})
	}
}

func TestSemanticDedup_OperationalDynamicProof(t *testing.T) {
	assert.InDelta(t, 1.0, NormalizedLevenshtein("Auth Service", "auth service"), 1e-9)
	assert.Greater(t, NormalizedLevenshtein("Auth Service", "Auth Services"), 0.85)
	assert.InDelta(t, 0.0, NormalizedLevenshtein("abc", ""), 1e-9)

	cfg := DedupConfig{SimilarityThreshold: 0.85, CosineWeight: 0.7, LexicalWeight: 0.3}
	engine, err := NewSemanticDedupEngine(cfg)
	require.NoError(t, err)

	entities := []SemanticEntity{
		{ID: "n-01", Label: "Auth Gateway", ParentBranch: "main", Embedding: []float64{0.90, 0.40, 0.10}},
		{ID: "n-02", Label: "Auth Gateways", ParentBranch: "main", Embedding: []float64{0.91, 0.39, 0.10}},
		{ID: "n-03", Label: "Billing Ledger", ParentBranch: "main", Embedding: []float64{0.10, 0.10, 0.98}},
	}
	for _, ent := range entities {
		require.NoError(t, engine.RegisterEntity(ent))
	}
	require.NoError(t, engine.AddEdge(SemanticEdge{FromID: "n-01", ToID: "n-03", Relation: "depends_on"}))
	require.NoError(t, engine.AddEdge(SemanticEdge{FromID: "n-02", ToID: "n-03", Relation: "depends_on"}))
	require.NoError(t, engine.AddEdge(SemanticEdge{FromID: "n-02", ToID: "n-01", Relation: "alias_of"}))

	report, err := engine.CollapseDuplicates()
	require.NoError(t, err)

	assert.Equal(t, 3, report.OriginalCount)
	assert.Equal(t, 2, report.CanonicalCount)
	assert.Equal(t, 1, report.CollapsedCount)
	assert.Equal(t, "n-01", report.RedirectMap["n-02"])
	require.Len(t, report.Clusters, 1)
	assert.Equal(t, "n-01", report.Clusters[0].CanonicalID)
	assert.Equal(t, []string{"n-02"}, report.Clusters[0].MergedIDs)
	assert.NotEmpty(t, report.Clusters[0].CanonicalCAS)
	assert.Equal(t, []SemanticEdge{{FromID: "n-01", ToID: "n-03", Relation: "depends_on"}}, report.RewiredEdges)

	// Concurrency proof via goroutinelabels (zero raw go func()).
	concEngine, err := NewSemanticDedupEngine(cfg)
	require.NoError(t, err)
	var regErrors atomic.Int64
	wg := goroutinelabels.StartTestGoroutines(t, 4, "dedup_worker", "concurrent entity registration", func(id int) {
		ent := SemanticEntity{
			ID:           fmt.Sprintf("conc-%02d", id),
			Label:        "Session Cache",
			ParentBranch: "main",
			Embedding:    []float64{0.85, 0.50, 0.05},
		}
		if regErr := concEngine.RegisterEntity(ent); regErr != nil {
			regErrors.Add(1)
		}
	})
	wg.Wait()
	assert.Zero(t, regErrors.Load())
	assert.Equal(t, 4, concEngine.EntityCount())

	concReport, err := concEngine.CollapseDuplicates()
	require.NoError(t, err)
	assert.Equal(t, 1, concReport.CanonicalCount)
	assert.Equal(t, 3, concReport.CollapsedCount)
}

func TestSemanticDedup_NegativeAdversarialBoundary(t *testing.T) {
	cfg := DedupConfig{SimilarityThreshold: 0.85, CosineWeight: 0.7, LexicalWeight: 0.3}
	engine, err := NewSemanticDedupEngine(cfg)
	require.NoError(t, err)

	// Distinct semantic signatures must never collapse.
	require.NoError(t, engine.RegisterEntity(SemanticEntity{
		ID:           "sig-a",
		Label:        "Vector Index",
		ParentBranch: "main",
		Embedding:    []float64{1.0, 0.0, 0.0},
	}))
	require.NoError(t, engine.RegisterEntity(SemanticEntity{
		ID:           "sig-b",
		Label:        "Policy Engine",
		ParentBranch: "main",
		Embedding:    []float64{0.0, 1.0, 0.0},
	}))
	// Identical embedding and label on a diverging ParentBranch must never collapse.
	require.NoError(t, engine.RegisterEntity(SemanticEntity{
		ID:           "sig-a-branch",
		Label:        "Vector Index",
		ParentBranch: "release/v2",
		Embedding:    []float64{1.0, 0.0, 0.0},
	}))

	// Duplicate IDs, dimension mismatches, and invalid edges must be rejected immediately.
	assert.ErrorIs(t, engine.RegisterEntity(SemanticEntity{
		ID:           "sig-a",
		Label:        "Vector Index",
		ParentBranch: "main",
		Embedding:    []float64{1.0, 0.0, 0.0},
	}), ErrEntityDuplicate)
	assert.ErrorIs(t, engine.RegisterEntity(SemanticEntity{
		ID:           "dim-bad",
		Label:        "Bad Dims",
		ParentBranch: "main",
		Embedding:    []float64{1.0, 0.0},
	}), ErrDimensionMismatch)
	assert.Error(t, engine.AddEdge(SemanticEdge{FromID: "", ToID: "sig-b", Relation: "rel"}))

	_, scoreErr := ScoreSimilarity(
		SemanticEntity{ID: "x", Label: "X", Embedding: []float64{1.0, 0.0}},
		SemanticEntity{ID: "y", Label: "Y", Embedding: []float64{1.0}},
		cfg,
	)
	assert.ErrorIs(t, scoreErr, ErrDimensionMismatch)

	report, err := engine.CollapseDuplicates()
	require.NoError(t, err)
	assert.Equal(t, 3, report.OriginalCount)
	assert.Equal(t, 3, report.CanonicalCount)
	assert.Equal(t, 0, report.CollapsedCount)
	assert.Empty(t, report.Clusters)
}
