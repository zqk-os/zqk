// BLI-STARTER-COMMUNITY-036 / PRI-STARTER-COMMUNITY-036 coverage elevation
package semanticcache

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/llm"
)

func TestSemanticCache_Delegates(t *testing.T) {
	c := NewSemanticCache(&mockLLMClient{intentReturn: "i"}, nil)
	ctx := context.Background()
	if _, err := c.AnalyzeVideoFrames(ctx, nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GenerateStructuredCompletion(ctx, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GenerateStructuredCompletionWithOptions(ctx, nil, nil, llm.StructuredCompletionOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.VerifyImage(ctx, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DescribeImage(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DescribeScene(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SemanticCompare(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}
}
