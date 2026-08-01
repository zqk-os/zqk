# Content-Addressable Storage Search Gap

**Version**: 1.0.0  
**Created**: 2026-01-05  
**Status**: Known Issue  
**Related**: Content-Addressable Storage Implementation, BLI-907

## Issue

The Search operation in `FileObjectStorage` does not properly search nested object fields (e.g., `metadata.description`). This causes the `TestContentAddressableStorage_ComprehensiveOperations/Search` test to fail.

## Current Behavior

The `searchObject` function in `pkg/storage/object_storage_file_search.go` only searches top-level string fields. When a field contains a nested object (like `metadata`), the search does not recurse into the nested structure.

### Example

```go
// Test object structure:
{
  "id": "TAM-400",
  "metadata": {
    "description": "test searchable content alpha"
  }
}

// Search query: "alpha"
// Expected: Find TAM-400 (alpha is in metadata.description)
// Actual: No results (search only checks top-level fields)
```

## Root Cause

In `searchObject` (line 122-215 of `object_storage_file_search.go`):

```go
// Current implementation only checks top-level fields
for key, value := range obj {
    if str, ok := value.(string); ok && len(str) > 0 {
        fieldsToSearch = append(fieldsToSearch, key)
    }
}
```

This logic:
1. Only considers top-level fields
2. Only searches string values directly
3. Does not recurse into nested objects/maps
4. Does not flatten nested structures for searching

## Impact

- **Test Coverage**: Search test fails, but this is a pre-existing limitation, not specific to content-addressable storage
- **Functionality**: Users cannot search within nested object fields
- **Workaround**: Search works for top-level string fields only

## Solution Options

### Option 1: Recursive Field Search (Recommended)
Modify `searchObject` to recursively search nested objects:

```go
func (f *FileObjectStorage) searchObjectRecursive(obj map[string]any, searchTerms []string, query SearchQuery, path string) {
    for key, value := range obj {
        currentPath := path
        if currentPath != "" {
            currentPath += "."
        }
        currentPath += key
        
        switch v := value.(type) {
        case string:
            // Search string value
            if strings.Contains(strings.ToLower(v), term) {
                // Match found
            }
        case map[string]any:
            // Recurse into nested object
            f.searchObjectRecursive(v, searchTerms, query, currentPath)
        case []any:
            // Search array elements
            for _, item := range v {
                if itemMap, ok := item.(map[string]any); ok {
                    f.searchObjectRecursive(itemMap, searchTerms, query, currentPath)
                }
            }
        }
    }
}
```

### Option 2: Flatten Objects for Search
Flatten nested objects into dot-notation keys (e.g., `metadata.description`) before searching.

### Option 3: Field-Specific Search Configuration
Allow spec files to define which nested fields should be searchable.

## Priority

**Low** - This is a pre-existing limitation in the search implementation, not a blocker for content-addressable storage integration. The search functionality works for top-level fields, which covers most use cases.

## Related Files

- `pkg/storage/object_storage_file_search.go` - Search implementation
- `pkg/storage/content_addressable_storage_comprehensive_test.go` - Test that exposes the gap
- `docs/architecture/README.md` - Content-addressable storage design

## Test Case

```go
// Test in content_addressable_storage_comprehensive_test.go
testObjects[0]["metadata"] = map[string]any{
    "description": "test searchable content alpha",
}

// Search query: "alpha"
// Expected: Find TAM-400
// Actual: No results (fails)
```

## Next Steps

1. Continue with content-addressable storage integration (current priority)
2. Address search gap in a future iteration
3. Consider adding recursive search as part of search enhancement backlog item

---

*This gap does not block content-addressable storage implementation. Search works for top-level fields, and nested field search can be enhanced separately.*

