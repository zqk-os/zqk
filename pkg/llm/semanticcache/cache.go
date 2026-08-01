package semanticcache

import (
	"context"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/llm"
	"github.com/lanceman/zqk/pkg/multimodal"
	"github.com/lanceman/zqk/pkg/storage"
)

// SemanticCache wraps an LLM client to intercept Generation calls,
// comparing intent via vector similarity in the MemGraph backend.
type SemanticCache struct {
	client  llm.Client
	storage storage.ObjectStorageProvider
}

// NewSemanticCache creates a new SemanticCache.
func NewSemanticCache(client llm.Client, storage storage.ObjectStorageProvider) *SemanticCache {
	return &SemanticCache{
		client:  client,
		storage: storage,
	}
}

// GenerateIntent generates an intent, using the cache if possible.
func (c *SemanticCache) GenerateIntent(ctx context.Context, code string) (string, error) {
	// First, generate an embedding for the input code
	embedding, err := c.client.GenerateEmbedding(ctx, code)
	if err != nil {
		return "", errfmt.Newf("failed to generate embedding for cache lookup").Wrap(err)
	}

	// Try to find a similar code block in the cache
	query := storage.Query{
		Type:       storage.QueryTypeVector,
		Expression: "MATCH (n:CodeEntity) WHERE vector.similarity(n.embedding, $vector) > $threshold RETURN n",
		Parameters: map[string]any{
			"vector":    embedding,
			"threshold": float32(0.95), // High threshold for cache hit
			"limit":     1,
		},
	}

	// Note: In a real implementation, we would need the security and storage contexts.
	// Since llm.Client doesn't receive them, we have to use empty ones or pass them in a different way.
	// For this phase, we assume the graph handles its own system-level auth for this internal operation.
	result, err := c.storage.Query(ctx, nil, nil, query)
	if err == nil && len(result.Objects) > 0 {
		// Cache hit!
		if intent, ok := result.Objects[0]["intent"].(string); ok && intent != "" {
			return intent, nil
		}
	}

	// Cache miss, generate intent using LLM
	return c.client.GenerateIntent(ctx, code)
}

// GenerateEmbedding just delegates to the underlying client.
func (c *SemanticCache) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	return c.client.GenerateEmbedding(ctx, text)
}

// AnalyzeVideoFrames delegates to the underlying client.
func (c *SemanticCache) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (multimodal.AnalysisResult, error) {
	return c.client.AnalyzeVideoFrames(ctx, frames, script)
}

// GenerateCompletion delegates to the underlying client.
func (c *SemanticCache) GenerateCompletion(ctx context.Context, prompt string, system string) (string, error) {
	return c.client.GenerateCompletion(ctx, prompt, system)
}

// GenerateStructuredCompletion delegates to the underlying client.
func (c *SemanticCache) GenerateStructuredCompletion(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (llm.StructuredCompletionResponse, error) {
	return c.client.GenerateStructuredCompletion(ctx, messages, tools)
}

// VerifyImage delegates to the underlying client.
func (c *SemanticCache) VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (multimodal.AnalysisResult, error) {
	return c.client.VerifyImage(ctx, image, referenceImage, textPrompt)
}

// DescribeImage delegates to the underlying client.
func (c *SemanticCache) DescribeImage(ctx context.Context, image []byte) (string, error) {
	return c.client.DescribeImage(ctx, image)
}

// DescribeScene delegates to the underlying client.
func (c *SemanticCache) DescribeScene(ctx context.Context, frames [][]byte) (string, error) {
	return c.client.DescribeScene(ctx, frames)
}

// SemanticCompare delegates to the underlying client.
func (c *SemanticCache) SemanticCompare(ctx context.Context, observedDescription string, expectedNarrative string) (float64, error) {
	return c.client.SemanticCompare(ctx, observedDescription, expectedNarrative)
}
