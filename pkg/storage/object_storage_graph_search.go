package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/objects"
)

// Search performs full-text search across objects (graph backend)
//
//nolint:gocritic // Interface requires value semantics for SearchQuery
func (g *GraphObjectStorage) Search(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query SearchQuery) (*SearchResult, error) {
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
		if err := g.checkPermission(secCtx, "read", kind); err != nil {
			return nil, err
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Build Cypher query for text search
	// For now, use simple CONTAINS matching (can be enhanced with full-text indexes later)
	cypherQuery, params := g.buildSearchCypherQuery(kindsToSearch, query)

	// Execute query
	providerQuery := provider.Query{
		Language: provider.QueryLanguageCypher,
		Query:    cypherQuery,
		Params:   params,
	}

	result, err := g.conn.ExecuteQuery(ctx, providerQuery)
	if err != nil {
		return nil, errfmt.Newf(ConstStreamFailedToExecuteSearchQuery).Wrap(err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Convert results to SearchMatch
	matches := g.convertSearchResults(result, query)

	// Sort by score (descending)
	sortSearchMatches(matches)

	// Apply pagination
	totalCount := len(matches)
	start := query.Offset
	if start < 0 {
		start = 0
	}
	end := start + query.Limit
	if query.Limit <= 0 {
		end = len(matches)
	}
	if end > len(matches) {
		end = len(matches)
	}

	var paginatedMatches []SearchMatch
	if start < len(matches) {
		paginatedMatches = matches[start:end]
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

// buildSearchCypherQuery builds a Cypher query for text search
//
//nolint:gocritic // Internal helper; query passed by value for consistency
func (g *GraphObjectStorage) buildSearchCypherQuery(kinds []string, query SearchQuery) (string, map[string]any) {
	params := make(map[string]any)
	searchTerms := parseSearchQuery(query.Query)
	params["searchTerms"] = searchTerms

	// Build label filter
	var labels []string
	for _, kind := range kinds {
		labels = append(labels, toLabel(kind))
	}

	labelFilter := ""
	if len(labels) > 0 {
		labelFilter = ":" + labels[0]
		for i := 1; i < len(labels); i++ {
			labelFilter += ":" + labels[i]
		}
	}

	// Build MATCH clause
	cypherQuery := fmt.Sprintf(ConstStreamMatchNstrEntity, labelFilter)

	// Build WHERE clause with text search
	conditions := []string{}
	for i, term := range searchTerms {
		paramName := fmt.Sprintf("term%d", i)
		params[paramName] = strings.ToLower(term)

		// Search in common text fields
		fieldConditions := []string{
			fmt.Sprintf(ConstStreamTolowerNTitleContainsStr, paramName),
			fmt.Sprintf(ConstStreamTolowerNDescriptionContainsStr, paramName),
			fmt.Sprintf(ConstStreamTolowerNBodyContainsStr, paramName),
		}

		// Add custom field searches if specified
		if len(query.Fields) > 0 {
			for _, field := range query.Fields {
				fieldConditions = append(fieldConditions, fmt.Sprintf(ConstStreamTolowerTostringNStrContainsStr, field, paramName))
			}
		}

		termCondition := "(" + strings.Join(fieldConditions, " OR ") + ")"
		conditions = append(conditions, termCondition)
	}

	if len(conditions) > 0 {
		cypherQuery += " WHERE " + strings.Join(conditions, " AND ")
	}

	// Calculate relevance score (simple: count of matching terms)
	scoreExpr := "0.0"
	for i := range searchTerms {
		paramName := fmt.Sprintf("term%d", i)
		scoreExpr += fmt.Sprintf(ConstStreamCaseWhenTolowerNTitleContainsStrOrTolowerN, paramName, paramName, paramName)
	}
	scoreExpr = fmt.Sprintf("(%s) / %d.0", scoreExpr, len(searchTerms))

	// RETURN clause
	cypherQuery += fmt.Sprintf(ConstStreamReturnNStrAsScore, scoreExpr)

	// Apply limit
	if query.Limit > 0 {
		cypherQuery += fmt.Sprintf(ConstStreamOrderByScoreDescLimitInt, query.Limit)
	} else {
		cypherQuery += ConstStreamOrderByScoreDesc
	}

	return cypherQuery, params
}

// convertSearchResults converts graph query results to SearchMatch
//
//nolint:gocritic // Internal helper; query passed by value for consistency
func (g *GraphObjectStorage) convertSearchResults(result *provider.QueryResult, query SearchQuery) []SearchMatch {
	var matches []SearchMatch

	// Process nodes
	for _, node := range result.Nodes {
		obj := g.nodeToObject(node)
		if obj == nil {
			continue
		}

		// Extract score from rows if available
		score := 0.5 // Default score
		if len(result.Rows) > 0 {
			for _, row := range result.Rows {
				// Find row that matches this node
				for _, value := range row {
					if n, ok := value.(*provider.Node); ok && n.ID == node.ID {
						// Extract score from row
						if scoreVal, ok := row[objects.FieldKeyScore].(float64); ok {
							score = scoreVal
						}
						break
					}
				}
			}
		}

		// Generate highlights if requested
		highlights := make(map[string][]string)
		matchedFields := []string{}
		if query.Highlight {
			searchTerms := parseSearchQuery(query.Query)
			for _, field := range []string{"title", "description", "body"} {
				if value := objects.GetString(obj, field); value != "" {
					for _, term := range searchTerms {
						if strings.Contains(strings.ToLower(value), term) {
							matchedFields = append(matchedFields, field)
							highlight := highlightMatch(value, term)
							if len(highlights[field]) == 0 {
								highlights[field] = []string{}
							}
							highlights[field] = append(highlights[field], highlight)
						}
					}
				}
			}
		}

		matches = append(matches, SearchMatch{
			Object:        obj,
			Score:         score,
			Highlights:    highlights,
			MatchedFields: matchedFields,
		})
	}

	return matches
}
