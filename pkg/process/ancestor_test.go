package process

import (
	"os"
	"testing"
)

func TestIsAncestorPID_selfAndParent(t *testing.T) {
	t.Parallel()
	self := os.Getpid()
	parent := os.Getppid()
	if !IsAncestorPID(self, self) {
		t.Fatal("process is an ancestor of itself")
	}
	if parent > 1 && !IsAncestorPID(parent, self) {
		t.Fatalf("parent %d should be an ancestor of %d", parent, self)
	}
	if parent > 1 && IsAncestorPID(self, parent) {
		t.Fatalf("child %d should not be an ancestor of parent %d", self, parent)
	}
}

func TestIsAncestorPID_unknown(t *testing.T) {
	t.Parallel()
	if IsAncestorPID(0, os.Getpid()) {
		t.Fatal("pid 0 is not an ancestor")
	}
	if IsAncestorPID(1_000_000_007, os.Getpid()) {
		t.Fatal("missing pid should not appear in the parent chain")
	}
}
