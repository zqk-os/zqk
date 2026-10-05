package crud

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/strutil"
)

// Search performs full-text search across objects (file backend)
func SearchObjects(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query SearchQuery, listFn func(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, ListFilter) (*QueryResult, error), permFn func(*pkgctx.SecurityContext, string, string) error) (*SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	kindsToSearch, err := PrepareSearchKinds(ctx, secCtx, query.Kinds, permFn)
	if err != nil {
		return nil, err
	}
	startTime := time.Now()

	// Perform search across all requested kinds
	var allMatches []SearchMatch
	for _, kind := range kindsToSearch {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		matches, err := searchKind(ctx, secCtx, storageCtx, kind, query, listFn)
		if err != nil {
			return nil, errfmt.Errorf("failed to search kind %s: %w", kind, err)
		}
		allMatches = append(allMatches, matches...)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Sort by score (descending)
	SortSearchMatches(allMatches)

	totalCount := len(allMatches)
	paginatedMatches := PaginateSearchMatches(allMatches, query.Offset, query.Limit)

	queryTime := time.Since(startTime)

	return &SearchResult{
		Objects:    paginatedMatches,
		TotalCount: totalCount,
		QueryTime:  queryTime.Nanoseconds(),
		Meta: map[string]any{
			"kinds_searched": kindsToSearch,
			"query":          query.Query,
		},
	}, nil
}

// searchKind searches within a specific object kind
//
//nolint:gocritic // Internal helper; query passed by value for consistency with interface
func searchKind(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, kind string, query SearchQuery, listFn func(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, ListFilter) (*QueryResult, error)) ([]SearchMatch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Use List to get all objects of this kind
	filter := ListFilter{
		Kind:  kind,
		Limit: 0, // Get all objects for search
	}

	listResult, err := listFn(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Search through objects (check context each iteration so large result sets can be cancelled)
	var matches []SearchMatch
	searchTerms := ParseSearchQueryWrapper(query.Query)

	for _, obj := range listResult.Objects {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		match := searchObject(obj, searchTerms, query)
		if match != nil && match.Score >= query.MinScore {
			matches = append(matches, *match)
		}
	}

	return matches, nil
}

// parseSearchQuery parses the search query into terms with synonym expansion
func ParseSearchQueryWrapper(query string) []string {
	terms := ParseSearchQuery(query)

	// Expand synonyms if enabled (basic implementation)
	// TODO: Load synonyms from storage and expand terms
	// For now, return terms as-is

	return terms
}

// searchObject searches within a single object
func searchObject(obj map[string]any, searchTerms []string, query SearchQuery) *SearchMatch {
	matchedFields := []string{}
	highlights := make(map[string][]string)
	totalScore := 0.0
	fieldCount := 0

	// Determine which fields to search
	fieldsToSearch := query.Fields
	if len(fieldsToSearch) == 0 {
		// Search string fields plus kernel object refs (often stored as *_refs lists).
		seen := make(map[string]struct{})
		for key, value := range obj {
			if str, ok := value.(string); ok && str != "" {
				fieldsToSearch = append(fieldsToSearch, key)
				seen[key] = struct{}{}
			}
			if objects.IsKernelObjectRefField(key) {
				if _, ok := seen[key]; ok {
					continue
				}
				fieldsToSearch = append(fieldsToSearch, key)
				seen[key] = struct{}{}
			}
		}
	}

	// Search each field
	for _, field := range fieldsToSearch {
		var fieldValue string
		if objects.IsKernelObjectRefField(field) {
			fieldValue = objects.KernelObjectRefGroupKey(obj, field)
			if fieldValue == "" {
				continue
			}
		} else {
			value, exists := obj[field]
			if !exists {
				continue
			}

			var ok bool
			fieldValue, ok = value.(string)
			if !ok {
				// Convert to string for searching
				fieldValue = fmt.Sprintf("%v", value)
			}
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
					highlight := HighlightMatchWrapper(fieldValue, term)
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
func HighlightMatchWrapper(text, term string) string {
	return HighlightMatch(text, term)
}

// SortSearchMatches sorts search matches by score (descending)
func SortSearchMatches(matches []SearchMatch) {
	sort.Slice(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })
}

// highlightMatch highlights matching text (shared helper)
func HighlightMatch(text, term string) string {
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
func ParseSearchQuery(query string) []string {
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
	return strutil.LevenshteinDistance(s1, s2)
}

func minInt(a, b, c int) int {
	return min(a, min(b, c))
}

// ResolveKindsToSearch returns queryKinds or all discoverable kinds if empty.
func ResolveKindsToSearch(queryKinds []string) ([]string, error) {
	if len(queryKinds) > 0 {
		return queryKinds, nil
	}
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		return nil, errfmt.Newf("failed to load field registry").Wrap(err)
	}
	allKinds, err := fieldRegistry.GetAllKinds()
	if err != nil {
		return nil, errfmt.Newf("failed to get all kinds").Wrap(err)
	}
	return allKinds, nil
}

// CheckReadPermissions checks read permission for each kind in the slice.
func CheckReadPermissions(ctx context.Context, secCtx *pkgctx.SecurityContext, kinds []string, permFn func(*pkgctx.SecurityContext, string, string) error) error {
	for _, kind := range kinds {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := permFn(secCtx, "read", kind); err != nil {
			return err
		}
	}
	return nil
}

// PrepareSearchKinds resolves kinds and checks read permissions for search.
func PrepareSearchKinds(ctx context.Context, secCtx *pkgctx.SecurityContext, kinds []string, permFn func(*pkgctx.SecurityContext, string, string) error) ([]string, error) {
	kindsToSearch, err := ResolveKindsToSearch(kinds)
	if err != nil {
		return nil, err
	}
	if err := CheckReadPermissions(ctx, secCtx, kindsToSearch, permFn); err != nil {
		return nil, err
	}
	return kindsToSearch, nil
}

// PaginateSearchMatches applies pagination offsets and limits to matches.
func PaginateSearchMatches(matches []SearchMatch, offset, limit int) []SearchMatch {
	start := offset
	if start < 0 {
		start = 0
	}
	end := start + limit
	if limit <= 0 || end > len(matches) {
		end = len(matches)
	}
	if start >= len(matches) {
		return nil
	}
	return matches[start:end]
}
