package testdiscovery_test

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/testdiscovery"
)

func TestPythonDiscoverer_CanHandleAndDiscover(t *testing.T) {
	t.Parallel()

	d := &testdiscovery.PythonDiscoverer{}
	if d.Language() != "python" {
		t.Errorf("expected python language, got %q", d.Language())
	}

	if !d.CanHandle("tests/test_math.py") {
		t.Errorf("expected PythonDiscoverer to handle test_*.py")
	}
	if d.CanHandle("src/math.go") {
		t.Errorf("PythonDiscoverer should not handle .go file")
	}

	code := `
def test_add():
    assert 1 + 1 == 2
`
	targets, err := d.Discover(context.Background(), ".", "tests/test_math.py", []byte(code))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Function != "test_add" {
		t.Errorf("expected test_add, got %q", targets[0].Function)
	}
}
