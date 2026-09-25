package intake

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// IntakeMembrane processes raw incoming work requests, enforces anti-1:1 micro-chaining,
// clusters related tasks semantically, and synthesizes concurrency topologies.
type IntakeMembrane struct {
	similarityThreshold float64
	strictAntiChain     bool
}

// Option configures an IntakeMembrane instance.
type Option func(*IntakeMembrane)

// WithSimilarityThreshold sets the Jaccard similarity threshold for clustering.
func WithSimilarityThreshold(threshold float64) Option {
	return func(m *IntakeMembrane) {
		m.similarityThreshold = threshold
	}
}

// WithStrictAntiChain toggles whether redundant 1:1 proposals are rejected fail-closed.
func WithStrictAntiChain(strict bool) Option {
	return func(m *IntakeMembrane) {
		m.strictAntiChain = strict
	}
}

// NewIntakeMembrane creates a configured IntakeMembrane.
func NewIntakeMembrane(opts ...Option) *IntakeMembrane {
	m := &IntakeMembrane{
		similarityThreshold: SimilarityThreshold,
		strictAntiChain:     true,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// ProcessIntake executes the two-stage intake membrane pipeline.
func (m *IntakeMembrane) ProcessIntake(requests []IntakeRequest) (*IntakeSynthesisResult, error) {
	if len(requests) == 0 {
		return nil, ErrEmptyIntakeRequests
	}

	// Validate operational scopes
	for _, req := range requests {
		if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.Title) == "" {
			return nil, ErrMissingOperationalScope
		}
	}

	// Stage 1: Semantic Clustering
	clusters, err := m.ClusterRequests(requests)
	if err != nil {
		return nil, err
	}

	// Negative Invariant: Reject 1:1 blind chaining
	// If requests > 2 and all resulted in singleton 1:1 clusters sharing domains, reject.
	if m.strictAntiChain && len(requests) >= 3 && len(clusters) == len(requests) {
		domainMap := make(map[string]int)
		for _, req := range requests {
			if req.DomainCategory != "" {
				domainMap[req.DomainCategory]++
			}
		}
		for _, count := range domainMap {
			if count >= 2 {
				return nil, fmt.Errorf("%w: detected %d fragmented singletons under domain", ErrRedundantOneToOneChains, count)
			}
		}
	}

	// Stage 2: Scrutinize Overlap & Synthesize Execution Topologies
	topologies := make([]*ExecutionTopology, len(clusters))
	for i, cluster := range clusters {
		topo, err := m.SynthesizeTopology(cluster)
		if err != nil {
			return nil, err
		}
		topologies[i] = topo
	}

	return &IntakeSynthesisResult{
		Clusters:          clusters,
		Topologies:        topologies,
		DeduplicatedCount: len(requests) - len(clusters),
		TotalInputCount:   len(requests),
	}, nil
}

// ClusterRequests groups requests based on domain and text token Jaccard similarity.
func (m *IntakeMembrane) ClusterRequests(requests []IntakeRequest) ([]*ClusteredWorkstream, error) {
	if len(requests) == 0 {
		return nil, ErrEmptyIntakeRequests
	}

	// Tokenize each request
	type tokenizedReq struct {
		request IntakeRequest
		tokens  map[string]struct{}
	}

	items := make([]tokenizedReq, len(requests))
	for i, req := range requests {
		items[i] = tokenizedReq{
			request: req,
			tokens:  extractTokens(req.Title + " " + req.Description + " " + strings.Join(req.TargetPaths, " ")),
		}
	}

	var clusters []*ClusteredWorkstream
	assigned := make([]bool, len(items))

	for i := 0; i < len(items); i++ {
		if assigned[i] {
			continue
		}

		clusterReqs := []IntakeRequest{items[i].request}
		assigned[i] = true
		clusterTokens := items[i].tokens

		for j := i + 1; j < len(items); j++ {
			if assigned[j] {
				continue
			}

			// Same domain category strongly encourages clustering
			sameDomain := items[i].request.DomainCategory != "" &&
				items[i].request.DomainCategory == items[j].request.DomainCategory

			sim := jaccardSimilarity(clusterTokens, items[j].tokens)

			// Share paths check
			sharesPath := hasPathOverlap(items[i].request.TargetPaths, items[j].request.TargetPaths)

			if sim >= m.similarityThreshold || (sameDomain && (sim >= 0.25 || sharesPath)) {
				clusterReqs = append(clusterReqs, items[j].request)
				assigned[j] = true
				// Merge tokens
				for tok := range items[j].tokens {
					clusterTokens[tok] = struct{}{}
				}
			}
		}

		// Compute common target paths
		commonPaths := findCommonPaths(clusterReqs)

		clusters = append(clusters, &ClusteredWorkstream{
			ClusterID:      fmt.Sprintf("CW-%s-%02d", items[i].request.DomainCategory, len(clusters)+1),
			Title:          fmt.Sprintf("Clustered Stream: %s (%d items)", clusterReqs[0].Title, len(clusterReqs)),
			DomainCategory: clusterReqs[0].DomainCategory,
			Requests:       clusterReqs,
			CommonPaths:    commonPaths,
		})
	}

	return clusters, nil
}

// SynthesizeTopology analyzes file and package overlap to partition tasks into concurrent vs sequential stages.
func (m *IntakeMembrane) SynthesizeTopology(cluster *ClusteredWorkstream) (*ExecutionTopology, error) {
	if cluster == nil || len(cluster.Requests) == 0 {
		return nil, ErrEmptyIntakeRequests
	}

	n := len(cluster.Requests)
	if n == 1 {
		return &ExecutionTopology{
			Mode:         ModeSequential,
			Batches:      [][]string{{cluster.Requests[0].ID}},
			Dependencies: nil,
			TotalItems:   1,
		}, nil
	}

	// Build adjacency overlap matrix
	// If two requests touch any overlapping file, they cannot run concurrently.
	hasConflict := false
	deps := make([]StageDependency, 0)

	// Map of request index to conflicting previous indices
	conflictMap := make(map[int][]int)

	for i := 0; i < n; i++ {
		for j := 0; j < i; j++ {
			if hasPathOverlap(cluster.Requests[i].TargetPaths, cluster.Requests[j].TargetPaths) {
				hasConflict = true
				conflictMap[i] = append(conflictMap[i], j)
				deps = append(deps, StageDependency{
					RequestID: cluster.Requests[i].ID,
					DependsOn: []string{cluster.Requests[j].ID},
				})
			}
		}
	}

	// If no conflicts exist, all items can execute in parallel (ModeConcurrent)
	if !hasConflict {
		batch := make([]string, n)
		for i, req := range cluster.Requests {
			batch[i] = req.ID
		}
		return &ExecutionTopology{
			Mode:         ModeConcurrent,
			Batches:      [][]string{batch},
			Dependencies: nil,
			TotalItems:   n,
		}, nil
	}

	// If every consecutive item conflicts, it's ModeSequential
	allSequential := true
	for i := 1; i < n; i++ {
		if len(conflictMap[i]) == 0 {
			allSequential = false
			break
		}
	}

	if allSequential {
		batches := make([][]string, n)
		for i, req := range cluster.Requests {
			batches[i] = []string{req.ID}
		}
		return &ExecutionTopology{
			Mode:         ModeSequential,
			Batches:      batches,
			Dependencies: deps,
			TotalItems:   n,
		}, nil
	}

	// Hybrid DAG: Partition into topological stages (levels)
	// Level 0: items with no dependencies
	levels := make([][]string, 0)
	assignedLevel := make(map[int]int)

	for i := 0; i < n; i++ {
		maxDepLevel := -1
		for _, depIdx := range conflictMap[i] {
			if lvl, ok := assignedLevel[depIdx]; ok && lvl > maxDepLevel {
				maxDepLevel = lvl
			}
		}
		itemLevel := maxDepLevel + 1
		assignedLevel[i] = itemLevel

		for len(levels) <= itemLevel {
			levels = append(levels, []string{})
		}
		levels[itemLevel] = append(levels[itemLevel], cluster.Requests[i].ID)
	}

	return &ExecutionTopology{
		Mode:         ModeHybridDAG,
		Batches:      levels,
		Dependencies: deps,
		TotalItems:   n,
	}, nil
}

func extractTokens(text string) map[string]struct{} {
	tokens := make(map[string]struct{})
	f := func(c rune) bool {
		return !unicode.IsLetter(c) && !unicode.IsNumber(c)
	}
	words := strings.FieldsFunc(strings.ToLower(text), f)
	stopwords := map[string]bool{
		"the": true, "and": true, "for": true, "with": true, "that": true,
		"this": true, "from": true, "into": true, "over": true, "under": true,
	}

	for _, w := range words {
		if len(w) >= 3 && !stopwords[w] {
			tokens[w] = struct{}{}
		}
	}
	return tokens
}

func jaccardSimilarity(a, b map[string]struct{}) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}

	intersection := 0
	for k := range a {
		if _, ok := b[k]; ok {
			intersection++
		}
	}

	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

func hasPathOverlap(pathsA, pathsB []string) bool {
	setA := make(map[string]struct{}, len(pathsA))
	for _, p := range pathsA {
		setA[p] = struct{}{}
	}
	for _, p := range pathsB {
		if _, ok := setA[p]; ok {
			return true
		}
	}
	return false
}

func findCommonPaths(requests []IntakeRequest) []string {
	if len(requests) == 0 {
		return nil
	}
	pathCounts := make(map[string]int)
	for _, r := range requests {
		seenInReq := make(map[string]bool)
		for _, p := range r.TargetPaths {
			if !seenInReq[p] {
				pathCounts[p]++
				seenInReq[p] = true
			}
		}
	}

	var common []string
	for path, count := range pathCounts {
		if count > 1 {
			common = append(common, path)
		}
	}
	sort.Strings(common)
	return common
}
