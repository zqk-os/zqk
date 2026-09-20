package swarminit

import "testing"

func TestPackage_defaultRegistryReady(t *testing.T) {
	t.Parallel()
	if DefaultRegistry() == nil {
		t.Fatal("DefaultRegistry returned nil")
	}
}
