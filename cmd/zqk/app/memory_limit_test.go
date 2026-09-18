package app

import (
	"runtime/debug"
	"testing"
)

func TestNFRMemoryLimit(t *testing.T) {
	// The memory limit should be 300MB to satisfy REQ-1785165360486952000-c1dfac70
	limit := debug.SetMemoryLimit(-1)
	expected := int64(300 * 1024 * 1024)
	if limit != expected {
		t.Errorf("expected memory limit to be %d (300MB), got %d", expected, limit)
	}
}
