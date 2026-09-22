// BLI-STARTER-COMMUNITY-036 / PRI-STARTER-COMMUNITY-036 coverage elevation
package rollback

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestCapture_EmptyStatesAndGetError(t *testing.T) {
	if config.StorageRollbackCaptureDisabled().Safe() {
		t.Skip("capture disabled")
	}
	dir := t.TempDir()
	id, err := Capture(dir, ScopeTypeMigration, "m1", func() ([]ObjectState, error) { return nil, nil })
	if err != nil || id != "" {
		t.Fatalf("empty states %q %v", id, err)
	}
	if _, err := Capture(dir, ScopeTypeMigration, "m1", func() ([]ObjectState, error) {
		return nil, errors.New("boom")
	}); err == nil {
		t.Fatal("getStates err")
	}
	if err := Retain(dir); err != nil {
		t.Fatal(err)
	}
	if p, err := Get(dir, "missing"); p != nil || (err == nil && p != nil) {
		t.Fatalf("missing get %v %v", p, err)
	}
	if err := Apply(nil, dir, "missing", storage.NewNoopObjectStorage()); err == nil {
		t.Fatal("apply missing")
	}
	if config.StorageRollbackCaptureDisabled().Safe() {
		return
	}
	id, err = Capture(dir, ScopeTypeMigration, "m2", func() ([]ObjectState, error) {
		return []ObjectState{{Kind: "backlog_item", ID: "BLI-1", State: map[string]any{"id": "BLI-1"}}}, nil
	})
	if err != nil || id == "" {
		t.Fatalf("capture %q %v", id, err)
	}
	if err := Apply(context.Background(), dir, id, storage.NewNoopObjectStorage()); err == nil {
		t.Fatal("noop apply")
	}
	if err := ApplyReconstruct(context.Background(), dir, time.Now(), []ObjectRef{{Kind: "backlog_item", ID: "BLI-1"}}, storage.NewNoopObjectStorage(), nil); err == nil {
		t.Fatal("reconstruct read")
	}
	if err := ApplyReconstruct(nil, dir, time.Now(), nil, storage.NewNoopObjectStorage(), nil); err != nil {
		t.Fatal(err)
	}
	if err := ApplyReconstruct(context.Background(), dir, time.Now(), []ObjectRef{{}}, storage.NewNoopObjectStorage(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := List(dir, 1, time.Hour); err != nil {
		t.Fatal(err)
	}
	_ = DefaultConfig()
	_ = ScopeTypeLifecycle
}
