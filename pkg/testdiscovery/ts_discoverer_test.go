package testdiscovery_test

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/testdiscovery"
)

func TestTSDiscoverer_CanHandleAndDiscover(t *testing.T) {
	t.Parallel()

	d := testdiscovery.NewTypeScriptDiscoverer()
	if d.Language() != "typescript" {
		t.Errorf("expected typescript language, got %q", d.Language())
	}

	if !d.CanHandle("src/app.test.ts") {
		t.Errorf("expected TSDiscoverer to handle .test.ts")
	}
	if d.CanHandle("src/app.go") {
		t.Errorf("TSDiscoverer should not handle .go file")
	}

	code := `
describe("math", () => {
    it("should add numbers", () => {
        expect(1 + 1).toBe(2);
    });
});
`
	targets, err := d.Discover(context.Background(), ".", "src/app.test.ts", []byte(code))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Function != "math > should add numbers" {
		t.Errorf("expected 'math > should add numbers', got %q", targets[0].Function)
	}
}
