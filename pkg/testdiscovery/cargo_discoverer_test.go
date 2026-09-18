package testdiscovery_test

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/testdiscovery"
)

func TestCargoDiscoverer_CanHandleAndDiscover(t *testing.T) {
	t.Parallel()

	d := &testdiscovery.CargoDiscoverer{}
	if d.Language() != "rust" {
		t.Errorf("expected rust language, got %q", d.Language())
	}

	if !d.CanHandle("src/lib_test.rs") {
		t.Errorf("expected CargoDiscoverer to handle .rs file")
	}
	if d.CanHandle("src/lib.go") {
		t.Errorf("CargoDiscoverer should not handle .go file")
	}

	code := `
#[test]
// Validates: CRIT-RUST-001
fn test_addition() {
    assert_eq!(1 + 1, 2);
}
`
	targets, err := d.Discover(context.Background(), ".", "src/test.rs", []byte(code))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) == 0 {
		t.Fatalf("expected discovered test target")
	}
	if targets[0].Function != "test_addition" {
		t.Errorf("expected function test_addition, got %q", targets[0].Function)
	}
}
