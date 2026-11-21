package community

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/strutil"
)

const (
	defaultCosineWeight  = 0.7
	defaultLexicalWeight = 0.3
)

var (
	// ErrInvalidSimilarityThreshold indicates a similarity threshold outside (0, 1].
	ErrInvalidSimilarityThreshold = errors.New("similarity threshold must be finite and in (0, 1]")
	// ErrInvalidWeights indicates negative, non-finite, or zero-sum similarity weights.
	ErrInvalidWeights = errors.New("similarity weights must be finite, non-negative, and sum > 0")
	// ErrNonFiniteEmbedding indicates NaN or Inf values inside a semantic embedding vector.
	ErrNonFiniteEmbedding = errors.New("embedding vector must contain only finite values")
	// ErrEntityDuplicate indicates an entity ID is already registered in the engine.
	ErrEntityDuplicate = errors.New("entity ID already registered")
)

// SemanticEntity represents a knowledge graph node with a semantic embedding and lineage branch.
type SemanticEntity struct {
	ID           string    `json:"id"`
	Label        string    `json:"label"`
	ParentBranch string    `json:"parent_branch"`
	Embedding    []float64 `json:"embedding"`
	CASDigest    string    `json:"cas_digest,omitempty"`
}

// SemanticEdge represents a directed relationship between two knowledge graph entities.
type SemanticEdge struct {
	FromID   string `json:"from_id"`
	ToID     string `json:"to_id"`
	Relation string `json:"relation"`
}

// DedupConfig configures similarity scoring weights and collision thresholds.
type DedupConfig struct {
	SimilarityThreshold float64 `json:"similarity_threshold"`
	CosineWeight        float64 `json:"cosine_weight"`
	LexicalWeight       float64 `json:"lexical_weight"`
}

// CollapsedCluster records a canonical CAS entity and the duplicate IDs merged into it.
type CollapsedCluster struct {
	CanonicalID  string   `json:"canonical_id"`
	CanonicalCAS string   `json:"canonical_cas"`
	MergedIDs    []string `json:"merged_ids"`
	Score        float64  `json:"score"`
}

// DedupReport summarizes the deterministic results of a semantic deduplication pass.
type DedupReport struct {
	OriginalCount  int                `json:"original_count"`
	CanonicalCount int                `json:"canonical_count"`
	CollapsedCount int                `json:"collapsed_count"`
	Clusters       []CollapsedCluster `json:"clusters"`
	CanonicalNodes []SemanticEntity   `json:"canonical_nodes"`
	RewiredEdges   []SemanticEdge     `json:"rewired_edges"`
	RedirectMap    map[string]string  `json:"redirect_map"`
}

// SemanticDedupEngine performs thread-safe semantic deduplication and deterministic edge rewiring.
type SemanticDedupEngine struct {
	mu       sync.RWMutex
	cfg      DedupConfig
	dims     int
	entities map[string]SemanticEntity
	edges    []SemanticEdge
}

// ValidateSemanticEntity enforces structural integrity on entity IDs, labels, and embeddings.
func ValidateSemanticEntity(entity SemanticEntity) error {
	if err := strutil.ErrorOnBlank(entity.ID, "entity ID cannot be blank"); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(entity.Label, "entity label cannot be blank"); err != nil {
		return err
	}
	if len(entity.Embedding) == 0 {
		return ErrEmptyVector
	}
	for _, val := range entity.Embedding {
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return ErrNonFiniteEmbedding
		}
	}
	return nil
}

// ValidateDedupConfig enforces normalized similarity threshold bounds and valid scoring weights.
func ValidateDedupConfig(cfg DedupConfig) error {
	if math.IsNaN(cfg.SimilarityThreshold) || math.IsInf(cfg.SimilarityThreshold, 0) {
		return ErrInvalidSimilarityThreshold
	}
	if cfg.SimilarityThreshold <= 0.0 || cfg.SimilarityThreshold > 1.0 {
		return ErrInvalidSimilarityThreshold
	}
	if math.IsNaN(cfg.CosineWeight) || math.IsInf(cfg.CosineWeight, 0) || cfg.CosineWeight < 0 {
		return ErrInvalidWeights
	}
	if math.IsNaN(cfg.LexicalWeight) || math.IsInf(cfg.LexicalWeight, 0) || cfg.LexicalWeight < 0 {
		return ErrInvalidWeights
	}
	return nil
}

// NormalizedLevenshtein computes case-insensitive normalized lexical similarity in [0, 1].
func NormalizedLevenshtein(a, b string) float64 {
	ra := []rune(strings.ToLower(a))
	rb := []rune(strings.ToLower(b))
	la, lb := len(ra), len(rb)
	if la == 0 && lb == 0 {
		return 1.0
	}
	if la == 0 || lb == 0 {
		return 0.0
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = minInt(del, minInt(ins, sub))
		}
		prev, curr = curr, prev
	}

	maxLen := la
	if lb > maxLen {
		maxLen = lb
	}
	return 1.0 - float64(prev[lb])/float64(maxLen)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ScoreSimilarity computes the weighted combination of cosine and lexical similarity in [0, 1].
func ScoreSimilarity(a, b SemanticEntity, cfg DedupConfig) (float64, error) {
	if err := ValidateSemanticEntity(a); err != nil {
		return 0, fmt.Errorf("invalid source entity: %w", err)
	}
	if err := ValidateSemanticEntity(b); err != nil {
		return 0, fmt.Errorf("invalid target entity: %w", err)
	}
	if len(a.Embedding) != len(b.Embedding) {
		return 0, ErrDimensionMismatch
	}
	if err := ValidateDedupConfig(cfg); err != nil {
		return 0, err
	}

	wCos, wLex := resolveWeights(cfg)
	cos := math.Max(0.0, math.Min(1.0, CosineSimilarity(a.Embedding, b.Embedding)))
	lex := NormalizedLevenshtein(a.Label, b.Label)
	return (wCos*cos + wLex*lex) / (wCos + wLex), nil
}

func resolveWeights(cfg DedupConfig) (float64, float64) {
	if cfg.CosineWeight == 0 && cfg.LexicalWeight == 0 {
		return defaultCosineWeight, defaultLexicalWeight
	}
	return cfg.CosineWeight, cfg.LexicalWeight
}

// NewSemanticDedupEngine constructs a thread-safe deduplication engine with validated configuration.
func NewSemanticDedupEngine(cfg DedupConfig) (*SemanticDedupEngine, error) {
	if err := ValidateDedupConfig(cfg); err != nil {
		return nil, err
	}
	wCos, wLex := resolveWeights(cfg)
	cfg.CosineWeight = wCos
	cfg.LexicalWeight = wLex

	return &SemanticDedupEngine{
		cfg:      cfg,
		entities: make(map[string]SemanticEntity),
		edges:    make([]SemanticEdge, 0),
	}, nil
}

// RegisterEntity validates and stores a semantic entity with a deterministic CAS digest.
func (e *SemanticDedupEngine) RegisterEntity(entity SemanticEntity) error {
	if err := ValidateSemanticEntity(entity); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.entities[entity.ID]; exists {
		return ErrEntityDuplicate
	}
	if e.dims > 0 && len(entity.Embedding) != e.dims {
		return ErrDimensionMismatch
	}
	if e.dims == 0 {
		e.dims = len(entity.Embedding)
	}

	copied := entity
	copied.Embedding = append([]float64(nil), entity.Embedding...)
	if copied.CASDigest == "" {
		copied.CASDigest = computeEntityCASDigest(copied)
	}
	e.entities[copied.ID] = copied
	return nil
}

// AddEdge validates and records a directed relationship edge between entities.
func (e *SemanticDedupEngine) AddEdge(edge SemanticEdge) error {
	for _, check := range []struct {
		value string
		field string
	}{
		{value: edge.FromID, field: "from_id"},
		{value: edge.ToID, field: "to_id"},
		{value: edge.Relation, field: "relation"},
	} {
		if err := strutil.ErrorOnBlank(check.value, "edge %s cannot be blank", check.field); err != nil {
			return err
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.edges = append(e.edges, edge)
	return nil
}

// EntityCount returns the number of registered entities.
func (e *SemanticDedupEngine) EntityCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.entities)
}

// CollapseDuplicates deterministically collapses synonymic duplicates into canonical CAS nodes and rewires edges.
func (e *SemanticDedupEngine) CollapseDuplicates() (*DedupReport, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	ids := make([]string, 0, len(e.entities))
	for id := range e.entities {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	canonicals, clusters, redirect, minScores, err := e.clusterEntities(ids)
	if err != nil {
		return nil, err
	}

	rewired := rewireEdges(e.edges, redirect)
	return buildDedupReport(len(ids), canonicals, clusters, redirect, minScores, rewired), nil
}

func (e *SemanticDedupEngine) clusterEntities(
	ids []string,
) ([]SemanticEntity, map[string][]string, map[string]string, map[string]float64, error) {
	canonicals := make([]SemanticEntity, 0, len(ids))
	clusters := make(map[string][]string)
	redirect := make(map[string]string, len(ids))
	minScores := make(map[string]float64)

	for _, id := range ids {
		candidate := e.entities[id]
		matchedID, score, err := e.findCanonicalMatch(candidate, canonicals)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if matchedID != "" {
			redirect[id] = matchedID
			clusters[matchedID] = append(clusters[matchedID], id)
			if prev, ok := minScores[matchedID]; !ok || score < prev {
				minScores[matchedID] = score
			}
			continue
		}
		canonicals = append(canonicals, candidate)
		redirect[id] = id
	}
	return canonicals, clusters, redirect, minScores, nil
}

func (e *SemanticDedupEngine) findCanonicalMatch(
	candidate SemanticEntity,
	canonicals []SemanticEntity,
) (string, float64, error) {
	for _, canon := range canonicals {
		if candidate.ParentBranch != canon.ParentBranch {
			continue
		}
		score, err := ScoreSimilarity(candidate, canon, e.cfg)
		if err != nil {
			return "", 0, err
		}
		if score >= e.cfg.SimilarityThreshold {
			return canon.ID, score, nil
		}
	}
	return "", 0, nil
}

func rewireEdges(edges []SemanticEdge, redirect map[string]string) []SemanticEdge {
	seen := make(map[SemanticEdge]struct{}, len(edges))
	rewired := make([]SemanticEdge, 0, len(edges))

	for _, edge := range edges {
		from := resolveRedirect(edge.FromID, redirect)
		to := resolveRedirect(edge.ToID, redirect)
		if from == to && edge.FromID != edge.ToID {
			continue
		}
		next := SemanticEdge{FromID: from, ToID: to, Relation: edge.Relation}
		if _, exists := seen[next]; exists {
			continue
		}
		seen[next] = struct{}{}
		rewired = append(rewired, next)
	}

	sort.Slice(rewired, func(i, j int) bool {
		if rewired[i].FromID != rewired[j].FromID {
			return rewired[i].FromID < rewired[j].FromID
		}
		if rewired[i].ToID != rewired[j].ToID {
			return rewired[i].ToID < rewired[j].ToID
		}
		return rewired[i].Relation < rewired[j].Relation
	})
	return rewired
}

func resolveRedirect(id string, redirect map[string]string) string {
	if target, ok := redirect[id]; ok {
		return target
	}
	return id
}

func buildDedupReport(
	originalCount int,
	canonicals []SemanticEntity,
	clusters map[string][]string,
	redirect map[string]string,
	minScores map[string]float64,
	rewired []SemanticEdge,
) *DedupReport {
	clusterList := make([]CollapsedCluster, 0, len(clusters))
	collapsedCount := 0
	for _, canon := range canonicals {
		merged := clusters[canon.ID]
		if len(merged) == 0 {
			continue
		}
		sort.Strings(merged)
		collapsedCount += len(merged)
		clusterList = append(clusterList, CollapsedCluster{
			CanonicalID:  canon.ID,
			CanonicalCAS: canon.CASDigest,
			MergedIDs:    merged,
			Score:        minScores[canon.ID],
		})
	}

	return &DedupReport{
		OriginalCount:  originalCount,
		CanonicalCount: len(canonicals),
		CollapsedCount: collapsedCount,
		Clusters:       clusterList,
		CanonicalNodes: canonicals,
		RewiredEdges:   rewired,
		RedirectMap:    redirect,
	}
}

func computeEntityCASDigest(entity SemanticEntity) string {
	var b strings.Builder
	b.WriteString(entity.ParentBranch)
	b.WriteByte('|')
	b.WriteString(strings.ToLower(entity.Label))
	for _, v := range entity.Embedding {
		fmt.Fprintf(&b, "|%.6f", v)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return "cas:" + hex.EncodeToString(sum[:16])
}
