# Ontology Versioning Layer: Architecture Design

**Last Verified:** 2026-08-31


## Overview
The Ontology Versioning Layer introduces context-awareness to the ZQK Knowledge Kernel. It allows the graph backend to support multiple concurrent ontology layers, enabling agents to operate within distinct structural contexts (e.g., `v1.0-stable`, `v1.1-experimental`).

## Storage Strategy
Each graph node and edge will be augmented with a `version_context` property, identifying the layer(s) to which it belongs.

- **Layer Identity**: A new node type `OntologyLayer` will exist, capturing the metadata, schema version, and active status.
- **Node/Edge Versioning**: Nodes and edges will be tagged with a list of `layer_ids`.

## API Contract
```go
// OntologyManager manages layer-aware traversals and queries.
type OntologyManager interface {
    // ListLayers returns all available ontology layers.
    ListLayers(ctx context.Context) ([]LayerInfo, error)

    // SetActiveLayer scopes the session to a specific layer.
    SetActiveLayer(ctx context.Context, layerID string) error

    // TraverseScoped executes graph operations restricted to the active layer.
    TraverseScoped(ctx context.Context, q TraversalQuery) (*QueryResult, error)
}
```

## Migration Path
1. **Schema Update**: Add `version_context` index to all graph elements.
2. **Layer Initialization**: Bootstrap a `default` layer containing all current nodes/edges.
3. **Transition**: Implement the `OntologyManager` to intercept and rewrite graph queries (Cypher) to include `WHERE version_context CONTAINS active_layer` filters.
