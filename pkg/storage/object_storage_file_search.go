package storage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

// Search performs full-text search across objects (file backend)
//
//nolint:gocritic // Interface requires value semantics for SearchQuery
func (f *FileObjectStorage) Search(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query SearchQuery) (*SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	startTime := time.Now()

	// Check permissions for requested kinds (or all kinds if empty)
	kindsToSearch := query.Kinds
	if len(kindsToSearch) == 0 {
		// Get all discoverable kinds
		fieldRegistry := objects.GetGlobalFieldRegistry()
		if err := fieldRegistry.LoadFields(); err != nil {
			return nil, errfmt.Newf(ConstStreamFailedToLoadFieldRegistry).Wrap(err)
		}
		allKinds, err := fieldRegistry.GetAllKinds()
		if err != nil {
			return nil, errfmt.Newf(ConstStreamFailedToGetAllKinds).Wrap(err)
		}
		kindsToSearch = allKinds
	}

	// Check permissions for each kind
	for _, kind := range kindsToSearch {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := f.checkPermission(secCtx, "read", kind); err != nil {
			return nil, err
		}
	}

	// Perform search across all requested kinds
	var allMatches []SearchMatch
	for _, kind := range kindsToSearch {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		matches, err := f.searchKind(ctx, secCtx, storageCtx, kind, query)
		if err != nil {
			return nil, errfmt.Errorf(ConstStreamFailedToSearchKindStrErr, kind, err)
		}
		allMatches = append(allMatches, matches...)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Sort by score (descending)
	sortSearchMatches(allMatches)

	// Apply pagination
	totalCount := len(allMatches)
	start := query.Offset
	if start < 0 {
		start = 0
	}
	end := start + query.Limit
	if query.Limit <= 0 {
		end = len(allMatches)
	}
	if end > len(allMatches) {
		end = len(allMatches)
	}

	var paginatedMatches []SearchMatch
	if start < len(allMatches) {
		paginatedMatches = allMatches[start:end]
	}

	queryTime := time.Since(startTime)

	return &SearchResult{
		Objects:    paginatedMatches,
		TotalCount: totalCount,
		QueryTime:  queryTime.Nanoseconds(),
		Meta: map[string]any{
			ConstStreamKindsSearched: kindsToSearch,
			"query":                  query.Query,
		},
	}, nil
}

// searchKind searches within a specific object kind
//
//nolint:gocritic // Internal helper; query passed by value for consistency with interface
func (f *FileObjectStorage) searchKind(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, kind string, query SearchQuery) ([]SearchMatch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Use List to get all objects of this kind
	filter := ListFilter{
		Kind:  kind,
		Limit: 0, // Get all objects for search
	}

	listResult, err := f.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Search through objects (check context each iteration so large result sets can be cancelled)
	var matches []SearchMatch
	searchTerms := f.parseSearchQuery(query.Query)

	for _, obj := range listResult.Objects {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		match := f.searchObject(obj, searchTerms, query)
		if match != nil && match.Score >= query.MinScore {
			matches = append(matches, *match)
		}
	}

	return matches, nil
}

// parseSearchQuery parses the search query into terms with synonym expansion
func (f *FileObjectStorage) parseSearchQuery(query string) []string {
	terms := parseSearchQuery(query)

	// Expand synonyms if enabled (basic implementation)
	// TODO: Load synonyms from storage and expand terms
	// For now, return terms as-is

	return terms
}

// searchObject searches within a single object
func (f *FileObjectStorage) searchObject(obj map[string]any, searchTerms []string, query SearchQuery) *SearchMatch {
	matchedFields := []string{}
	highlights := make(map[string][]string)
	totalScore := 0.0
	fieldCount := 0

	// Determine which fields to search
	fieldsToSearch := query.Fields
	if len(fieldsToSearch) == 0 {
		// Search all text fields (heuristic: string fields)
		for key, value := range obj {
			if str, ok := value.(string); ok && str != emptyValue {
				fieldsToSearch = append(fieldsToSearch, key)
			}
		}
	}

	// Search each field
	for _, field := range fieldsToSearch {
		value, exists := obj[field]
		if !exists {
			continue
		}

		fieldValue, ok := value.(string)
		if !ok {
			// Convert to string for searching
			fieldValue = fmt.Sprintf("%v", value)
		}

		fieldValueLower := strings.ToLower(fieldValue)
		fieldScore := 0.0
		fieldMatched := false
		var fieldHighlights []string

		// Check each search term
		for _, term := range searchTerms {
			matched := false
			if query.Fuzzy {
				// Fuzzy matching: check if term is similar to any word in field
				words := strings.Fields(fieldValueLower)
				for _, word := range words {
					if fuzzyMatch(word, term) {
						matched = true
						fieldScore += 0.8 // Slightly lower score for fuzzy matches
						break
					}
				}
			} else if strings.Contains(fieldValueLower, term) {
				// Exact match
				matched = true
				fieldScore += 1.0
			}

			if matched {
				fieldMatched = true
				// Generate highlight if requested
				if query.Highlight {
					highlight := f.highlightMatch(fieldValue, term)
					fieldHighlights = append(fieldHighlights, highlight)
				}
			}
		}

		if fieldMatched {
			matchedFields = append(matchedFields, field)
			if query.Highlight && len(fieldHighlights) > 0 {
				highlights[field] = fieldHighlights
			}
			totalScore += fieldScore
			fieldCount++
		}
	}

	if len(matchedFields) == 0 {
		return nil
	}

	// Calculate final score (normalize to 0.0-1.0)
	finalScore := totalScore / float64(len(searchTerms)*fieldCount)
	if finalScore > 1.0 {
		finalScore = 1.0
	}

	return &SearchMatch{
		Object:        obj,
		Score:         finalScore,
		Highlights:    highlights,
		MatchedFields: matchedFields,
	}
}

// highlightMatch highlights matching text in a field value
func (f *FileObjectStorage) highlightMatch(text, term string) string {
	return highlightMatch(text, term)
}

// sortSearchMatches sorts search matches by score (descending)
func sortSearchMatches(matches []SearchMatch) {
	sort.Slice(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })
}

// highlightMatch highlights matching text (shared helper)
func highlightMatch(text, term string) string {
	lowerText := strings.ToLower(text)
	lowerTerm := strings.ToLower(term)

	if !strings.Contains(lowerText, lowerTerm) {
		return text
	}

	idx := strings.Index(lowerText, lowerTerm)
	if idx >= 0 {
		before := text[:idx]
		match := text[idx : idx+len(term)]
		after := text[idx+len(term):]
		return fmt.Sprintf("%s**%s**%s", before, match, after)
	}

	return text
}

// parseSearchQuery parses the search query into terms (shared helper)
func parseSearchQuery(query string) []string {
	return strings.Fields(strings.ToLower(query))
}

// fuzzyMatch performs simple fuzzy matching using Levenshtein distance
func fuzzyMatch(word, term string) bool {
	// Simple fuzzy matching: allow 1-2 character difference for short terms
	if len(term) <= 3 {
		// For very short terms, require exact match
		return word == term
	}

	// Calculate simple edit distance
	dist := levenshteinDistance(word, term)
	maxDist := len(term) / 3 // Allow up to 1/3 of term length as difference
	if maxDist < 1 {
		maxDist = 1
	}

	return dist <= maxDist
}

// levenshteinDistance calculates the Levenshtein distance between two strings
func levenshteinDistance(s1, s2 string) int {
	if s1 == emptyValue {
		return len(s2)
	}
	if s2 == emptyValue {
		return len(s1)
	}

	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
	}

	for i := 0; i <= len(s1); i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		matrix[0][j] = j
	}

	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}

			matrix[i][j] = minInt(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(s1)][len(s2)]
}

// minInt returns the minimum of three integers
func minInt(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
