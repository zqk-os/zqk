package storage

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
)

func TestCanonicalizePersistedLifecycleStatus_UnknownFailsClosed(t *testing.T) {
	t.Parallel()
	loader := objects.NewLifecycleLoader(filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir))
	_, err := canonicalizePersistedLifecycleStatus(loader, objects.KindTechnicalDebt, "accepted")
	if err == nil {
		t.Fatal("expected unknown lifecycle status to fail closed")
	}
	if !errors.Is(err, objects.ErrLifecycleStatusUnknown) {
		t.Fatalf("err=%v, want ErrLifecycleStatusUnknown", err)
	}
}

func TestCanonicalizePersistedLifecycleStatus_AliasOK(t *testing.T) {
	t.Parallel()
	loader := objects.NewLifecycleLoader(filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir))
	got, err := canonicalizePersistedLifecycleStatus(loader, objects.KindBacklogItem, "implemented")
	if err != nil {
		t.Fatal(err)
	}
	if got != objects.ObjectStatusComplete {
		t.Fatalf("got %q, want complete", got)
	}
}
