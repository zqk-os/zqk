package testdiscovery_test

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/testdiscovery"
)

func TestEngine_CreationAndOptions(t *testing.T) {
	t.Parallel()

	e := testdiscovery.NewEngine()
	if e == nil {
		t.Fatal("expected non-nil engine")
	}

	opts := testdiscovery.DiscoveryOptions{
		ProjectRoot: ".",
		Languages:   []string{"go"},
		Workers:     2,
	}

	targets, err := e.Discover(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected discovery error: %v", err)
	}
	if len(targets) == 0 {
		t.Log("no targets discovered in current root with given options")
	}
}
