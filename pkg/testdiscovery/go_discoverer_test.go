package testdiscovery_test

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/testdiscovery"
)

func TestGoDiscoverer_CanHandleAndDiscover(t *testing.T) {
	t.Parallel()

	d := &testdiscovery.GoDiscoverer{}
	if d.Language() != "go" {
		t.Errorf("expected go language, got %q", d.Language())
	}

	if !d.CanHandle("pkg/foo/bar_test.go") {
		t.Errorf("expected GoDiscoverer to handle *_test.go")
	}
	if d.CanHandle("pkg/foo/bar.go") {
		t.Errorf("GoDiscoverer should not handle non-test go file")
	}

	code := `package foo_test
import "testing"
// Validates: CRIT-GO-001
func TestSample(t *testing.T) {}
`
	targets, err := d.Discover(context.Background(), ".", "pkg/foo/bar_test.go", []byte(code))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Function != "TestSample" {
		t.Errorf("expected TestSample, got %q", targets[0].Function)
	}
}
