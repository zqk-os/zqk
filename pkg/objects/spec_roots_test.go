package objects

import (
	"path/filepath"
	"testing"
)

func TestExtraSpecRoots(t *testing.T) {
	dummy := filepath.Join(t.TempDir(), "dummy_specs")
	AddSpecRoot(dummy)
	t.Cleanup(func() {
		RemoveSpecRoot(dummy)
	})

	roots := ExtraSpecRoots()
	var found bool
	for _, r := range roots {
		if r == filepath.Clean(dummy) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected dummy root %s in %v", dummy, roots)
	}

	RemoveSpecRoot(dummy)
	rootsAfter := ExtraSpecRoots()
	for _, r := range rootsAfter {
		if r == filepath.Clean(dummy) {
			t.Fatalf("expected dummy root %s removed from %v", dummy, rootsAfter)
		}
	}
}

func TestExtraLifecycleRoots(t *testing.T) {
	dummy := filepath.Join(t.TempDir(), "dummy_lifecycles")
	AddLifecycleRoot(dummy)
	t.Cleanup(func() {
		RemoveLifecycleRoot(dummy)
	})

	roots := ExtraLifecycleRoots()
	var found bool
	for _, r := range roots {
		if r == filepath.Clean(dummy) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected dummy lifecycle root %s in %v", dummy, roots)
	}

	RemoveLifecycleRoot(dummy)
	rootsAfter := ExtraLifecycleRoots()
	for _, r := range rootsAfter {
		if r == filepath.Clean(dummy) {
			t.Fatalf("expected dummy lifecycle root %s removed from %v", dummy, rootsAfter)
		}
	}
}
