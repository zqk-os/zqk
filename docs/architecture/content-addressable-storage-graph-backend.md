# Content-Addressable Storage for Graph Backend

**Last Verified:** 2026-08-31


**Topology:** File CAS remains durability SSOT; graph stores projection properties (e.g. `content_hash`) when Memgraph is enabled — [ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md](./decisions/ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md).

## Confirmation: Yes, It Works!

Content-addressable storage principles apply to the graph backend, but the implementation differs from the file backend.

## Key Differences

### File Backend (Content-Addressable)
- **Storage**: `{hash}.yaml` files
- **Lookup**: Index `ID -> hash`, then read `{hash}.yaml`
- **Deduplication**: Same content = same file (multiple IDs can reference same file)
- **Integrity**: Hash IS the filename

### Graph Backend (Content-Addressable Principles)
- **Storage**: Graph nodes with `id` as node ID
- **Hash Storage**: Hash stored as `content_hash` property on node
- **Lookup**: Direct by node ID (ID is the node identifier)
- **Deduplication**: Can check if node with same hash exists before creating
- **Integrity**: Hash stored as property, verified on read

## Implementation for Graph Backend

### Create Operation

```go
func (g *GraphObjectStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
    // 1. Marshal object to determine content
    data, err := yaml.Marshal(obj)
    
    // 2. Calculate hash BEFORE storing (Git's approach)
    hash := CalculateSHA256Hash(data)
    
    // 3. Optional: Check for content deduplication
    // Query: MATCH (n {content_hash: $hash}) RETURN n
    // If exists, could reuse (but breaks ID-based model, so skip for now)
    
    // 4. Store hash as property on node
    obj["content_hash"] = hash
    
    // 5. Create node (ID is still object ID, not hash)
    node, err := g.objectToNode(obj)
    return g.conn.CreateNode(ctx, node)
}
```

### Read Operation

```go
func (g *GraphObjectStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
    // 1. Get node by ID
    node, err := g.conn.GetNode(ctx, id, []string{label, "Entity"})
    
    // 2. Convert to object
    obj := g.nodeToObject(node)
    
    // 3. Verify integrity (optional but recommended)
    storedHash := obj["content_hash"].(string)
    
    // Re-marshal to get current content
    data, _ := yaml.Marshal(obj)
    calculatedHash := CalculateSHA256Hash(data)
    
    if storedHash != calculatedHash {
        return nil, fmt.Errorf("hash mismatch: node may be corrupted")
    }
    
    return obj, nil
}
```

## Benefits for Graph Backend

1. **Integrity Verification**: Hash stored on node, verified on read
2. **Corruption Detection**: Can detect if node properties were modified
3. **Content Deduplication**: Can query for existing nodes with same hash
4. **Consistency**: Same principles as file backend, adapted for graph

## Differences from File Backend

| Aspect | File Backend | Graph Backend |
|--------|-------------|---------------|
| **Storage Key** | Hash (filename) | ID (node ID) |
| **Hash Location** | Filename | Node property |
| **Lookup** | Index `ID -> hash` | Direct by ID |
| **Deduplication** | Multiple IDs → same file | Query for existing hash |
| **Integrity** | Hash IS filename | Hash stored as property |

## Implementation Strategy

**Phase 1: Add Hash Property**
- Store `content_hash` on all nodes
- Calculate hash before storing (Git's approach)
- Verify hash on read

**Phase 2: Integrity Checks**
- Add hash verification to Read()
- Add hash validation to system check
- Detect corruption

**Phase 3: Content Deduplication (Optional)**
- Before creating, check if node with same hash exists
- Reuse existing node if content matches
- More complex (requires handling multiple IDs → one node)

## Conclusion

**Yes, content-addressable storage works for graph backend!**

The principles apply:
- ✅ Calculate hash before storing
- ✅ Store hash for integrity
- ✅ Verify hash on read
- ✅ Detect corruption

The implementation differs (property vs filename), but the benefits are the same:
- ✅ Integrity verification
- ✅ Corruption detection
- ✅ Consistent with file backend principles

