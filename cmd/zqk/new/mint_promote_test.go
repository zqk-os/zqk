package newcmd

import (
	"testing"
)

func TestEnqueueMintPromoteJob_requiresArgs(t *testing.T) {
	if _, err := enqueueMintPromoteJob("", "DOC-1"); err == nil {
		t.Fatal("expected error for empty project root")
	}
	if _, err := enqueueMintPromoteJob("/tmp", ""); err == nil {
		t.Fatal("expected error for empty object id")
	}
}
