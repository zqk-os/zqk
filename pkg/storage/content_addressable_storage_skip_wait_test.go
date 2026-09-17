package storage

import (
	"testing"

	"github.com/lanceman/zqk/pkg/storage/filecas"
)

func TestCAS_SkipIndexUpdateWaitAtomic(t *testing.T) {
	t.Parallel()

	initVal := filecas.GetSkipIndexUpdateWait()
	defer filecas.SetSkipIndexUpdateWait(initVal)

	filecas.SetSkipIndexUpdateWait(true)
	if !filecas.GetSkipIndexUpdateWait() {
		t.Fatalf("expected filecas.GetSkipIndexUpdateWait() to return true after Store(true)")
	}

	filecas.SetSkipIndexUpdateWait(false)
	if filecas.GetSkipIndexUpdateWait() {
		t.Fatalf("expected filecas.GetSkipIndexUpdateWait() to return false after Store(false)")
	}
}
