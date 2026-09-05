# Full-Text Search v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-12-29  
**Status**: Implemented  
**Related**: BLI-640, Gap #4, QUE-001, ADR-001

## Overview

This document defines the full-text search implementation for the zqk object storage system, enabling content-based search across all text fields in objects.

## Architecture Decision Record

**ADR**: [ADR-001](../decisions/ADR-001.yaml) (Architecture Decision - Lightweight Full-Text Search Syntax)  
**Status**: Active  
**Revisit**: 2026-12-30

## Decision (QUE-001)

**Selected Approach**: Lightweight syntax with stemming

**Rationale**:
- Aligns with architecture principles (minimal dependencies, simplicity)
- Graph backend can leverage native Cypher text search
- File backend uses lightweight implementation
- Can be enhanced later if needed
- No resource overhead or deployment complexity

## Requirements (from Gap #4)

1. **Search Method**: `Search(ctx, secCtx, query string, options SearchOptions) (*QueryResult, error)`
2. **Full-Text Search**: Across all text fields
3. **Fuzzy Matching**: Support for typos and variations
4. **Stemming**: Word root matching
5. **Synonyms**: Support for synonym expansion
6. **Search Highlighting**: Mark matching text in results

## API Design

### Search Method

```go
// Search performs full-text search across objects
// - Enforces permissions (read permission for object kinds)
// - Supports fuzzy matching, stemming, and synonyms
// - Returns matching objects with relevance scores
// - Supports highlighting of matching text
Search(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, query SearchQuery) (*SearchResult, error)
```

### Search Query

```go
// SearchQuery defines a full-text search query
type SearchQuery struct {
	Query       string   // Search query string
	Kinds       []string // Object kinds to search (empty = all)
	Fields      []string // Fields to search (empty = all text fields)
	Fuzzy       bool     // Enable fuzzy matching
	Stemming    bool     // Enable stemming
	Synonyms    bool     // Enable synonym expansion
	Highlight   bool     // Enable result highlighting
	Limit       int      // Maximum results
	Offset      int      // Pagination offset
	MinScore    float64  // Minimum relevance score (0.0-1.0)
}

// SearchResult contains search results with relevance scores
type SearchResult struct {
	Objects     []SearchMatch // Matching objects with scores
	TotalCount  int           // Total number of matches
	QueryTime   time.Duration // Query execution time
	Meta        map[string]any // Metadata
}

// SearchMatch represents a single search result
type SearchMatch struct {
	Object      map[string]any // The matching object
	Score       float64        // Relevance score (0.0-1.0)
	Highlights  map[string][]string // Field -> highlighted snippets
	MatchedFields []string     // Fields that matched
}
```

## Implementation Strategy

### File Backend

- **Lightweight Text Search**: Use Go's `strings` package and regex for basic matching
- **Fuzzy Matching**: Simple Levenshtein distance for typos
- **Stemming**: Use a lightweight Go stemming library (e.g., `github.com/kljensen/snowball`)
- **Synonyms**: Load from `kind_synonym` objects in the system
- **Indexing**: In-memory index built on first search (can be cached)
- **Performance**: For large datasets, consider building a simple inverted index

### Graph Backend

- **Cypher Text Search**: Use Cypher's native text search functions
- **Full-Text Index**: Leverage graph database full-text indexes
- **Fuzzy Matching**: Use Cypher fuzzy matching functions
- **Stemming**: Use graph database's built-in stemming
- **Synonyms**: Query `kind_synonym` nodes in the graph

## Search Syntax

### Basic Query
```
"search term"
```

### Multiple Terms
```
"term1 term2"  // AND (both terms must match)
```

### Phrase Search
```
"exact phrase"
```

### Field-Specific Search
```
title:"specific title" body:"content"
```

### Fuzzy Search
```
~term  // Fuzzy match (enabled via Fuzzy option)
```

## Examples

### Simple Search
```go
query := SearchQuery{
	Query: "backlog item",
	Kinds: []string{"backlog_item"},
	Limit: 10,
}
result, err := storage.Search(ctx, secCtx, storageCtx, query)
```

### Advanced Search with Highlighting
```go
query := SearchQuery{
	Query:     "priority plan",
	Kinds:     []string{"priority_plan", "backlog_item"},
	Fuzzy:     true,
	Stemming:  true,
	Synonyms:  true,
	Highlight: true,
	MinScore:  0.5,
	Limit:     20,
}
result, err := storage.Search(ctx, secCtx, storageCtx, query)
```

## Future Enhancements

1. **Persistent Index**: Build and maintain search index on disk
2. **Advanced Ranking**: TF-IDF, BM25 scoring
3. **Faceted Search**: Filter by categories, tags, etc.
4. **Autocomplete**: Search suggestions
5. **Elasticsearch Integration**: Optional backend for large-scale deployments

