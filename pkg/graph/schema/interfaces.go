package schema

import "context"

// Layer defines the semantic layer in the GraphRAG Three-Layer Architecture.
type Layer string

const (
	LayerDocument     Layer = "document"     // Layer 1: Raw data, documents, chunks, and metadata
	LayerEntity       Layer = "entity"       // Layer 2: Semantic entities (objects from System Ontology)
	LayerRelationship Layer = "relationship" // Layer 3: Explicit relationships between entities
)

// GraphRAGSchema defines the overarching Three-Layer Architecture for ZQK's GraphRAG implementation.
type GraphRAGSchema interface {
	GetDocumentLayer() DocumentLayer
	GetEntityLayer() EntityLayer
	GetRelationshipLayer() RelationshipLayer
}

// DocumentLayer handles the ingestion and management of raw source documents (Layer 1).
type DocumentLayer interface {
	AddDocument(ctx context.Context, doc Document) error
	GetDocument(ctx context.Context, id string) (*Document, error)
	AddChunk(ctx context.Context, chunk Chunk) error
	GetChunksByDocument(ctx context.Context, docID string) ([]Chunk, error)
}

// EntityLayer handles the semantic knowledge graph of entities (Layer 2).
type EntityLayer interface {
	AddEntity(ctx context.Context, entity Entity) error
	GetEntity(ctx context.Context, id string) (*Entity, error)
	UpdateEntity(ctx context.Context, id string, updates map[string]any) error
	SimilaritySearch(ctx context.Context, queryVector []float32, limit int) ([]SearchResult, error)
}

// RelationshipLayer handles explicit edges between entities (Layer 3).
type RelationshipLayer interface {
	AddRelationship(ctx context.Context, rel Relationship) error
	GetRelationships(ctx context.Context, entityID string) ([]Relationship, error)
	DeleteRelationship(ctx context.Context, sourceID, targetID, relType string) error
}
