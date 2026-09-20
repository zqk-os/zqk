package llm

import (
	"context"
	"testing"
)

func TestAnalyzeVideoFramesMock(t *testing.T) {
	config := &Config{
		BaseURL:   "https://api.openai.com/v1",
		APIKey:    "", // Mock mode
		ChatModel: "gpt-4o",
	}

	client := NewOpenAIClient(context.Background(), config)

	ctx := context.Background()
	frames := [][]byte{
		[]byte("frame1"),
		[]byte("frame2"),
	}
	script := "Test script"

	result, err := client.AnalyzeVideoFrames(ctx, frames, script)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TruthScore == 0 {
		t.Errorf("expected non-zero TruthScore in mock mode")
	}
	if result.Summary == "" {
		t.Errorf("expected non-empty Summary in mock mode")
	}
}

func TestDescribeImageMock(t *testing.T) {
	config := &Config{
		BaseURL:   "https://api.openai.com/v1",
		APIKey:    "", // Mock mode
		ChatModel: "gpt-4o",
	}

	client := NewOpenAIClient(context.Background(), config)
	ctx := context.Background()

	desc, err := client.DescribeImage(ctx, []byte("image"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc == "" {
		t.Errorf("expected non-empty description in mock mode")
	}
}

func TestSemanticCompareMock(t *testing.T) {
	config := &Config{
		BaseURL:    "https://api.openai.com/v1",
		APIKey:     "", // Mock mode
		EmbedModel: "text-embedding-3-small",
	}

	client := NewOpenAIClient(context.Background(), config)
	ctx := context.Background()

	score, err := client.SemanticCompare(ctx, "A red car", "A fast red automobile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// In mock mode, the embeddings are initialized to zero, so dot product is zero.
	// We just ensure it doesn't fail.
	if score != 0 {
		t.Logf("got score %f", score)
	}
}
