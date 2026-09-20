package schema

// Document represents a source file or document in the Document Layer.
type Document struct {
	ID       string
	URI      string
	Content  string
	Metadata map[string]any
}

// Chunk represents a segmented piece of a Document in the Document Layer.
type Chunk struct {
	ID         string
	DocumentID string
	Content    string
	StartIndex int
	EndIndex   int
}

// Entity represents a semantic node in the Entity Layer.
type Entity struct {
	ID         string
	Type       string
	Name       string
	Properties map[string]any
}

// Relationship represents an edge in the Relationship Layer.
type Relationship struct {
	SourceID   string
	TargetID   string
	Type       string
	Properties map[string]any
}

// SearchResult represents a result from the Entity Layer similarity search.
type SearchResult struct {
	ID    string
	Score float32
}
