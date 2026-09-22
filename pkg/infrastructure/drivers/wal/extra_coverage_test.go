// BLI-STARTER-COMMUNITY-044 / PRI-STARTER-COMMUNITY-044 coverage elevation
package wal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/infrastructure"
)

func TestExtraWALSpinePublishReplayClose(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "wal", "spine.log")
	spine, err := NewWALSpine(ctx, path, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = spine.Close() })

	ev := infrastructure.Event{ObjectID: "OBJ-1", Kind: "backlog_item", Op: "update"}
	if err := spine.Publish(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if err := spine.Publish(ctx, infrastructure.Event{ObjectID: "OBJ-2", Kind: "requirement", Op: "create"}); err != nil {
		t.Fatal(err)
	}
	if err := spine.Subscribe(ctx, "backlog_item", nil); err != nil {
		t.Fatal(err)
	}

	var seen []string
	if err := spine.Replay(ctx, 0, func(_ context.Context, event infrastructure.Event) error {
		seen = append(seen, event.ObjectID)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 {
		t.Fatalf("replay = %#v", seen)
	}
	seen = nil
	if err := spine.Replay(ctx, 1, func(_ context.Context, event infrastructure.Event) error {
		seen = append(seen, event.ObjectID)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0] != "OBJ-2" {
		t.Fatalf("skip seq = %#v", seen)
	}
	if err := spine.Replay(ctx, 0, func(context.Context, infrastructure.Event) error {
		return errors.New("stop")
	}); err == nil {
		t.Fatal("handler err")
	}

	if err := spine.Close(); err != nil {
		t.Fatal(err)
	}
	spine2, err := NewWALSpine(ctx, path, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = spine2.Close() })
	if err := spine2.Publish(ctx, infrastructure.Event{ObjectID: "OBJ-3", Kind: "goal", Op: "create"}); err != nil {
		t.Fatal(err)
	}

	garbage := filepath.Join(t.TempDir(), "g.log")
	if err := os.WriteFile(garbage, []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gspine, err := NewWALSpine(ctx, garbage, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gspine.Close() })
	if err := gspine.Replay(ctx, 0, func(context.Context, infrastructure.Event) error {
		t.Fatal("should skip garbage")
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	cwd := t.TempDir()
	t.Chdir(cwd)
	def, err := NewWALSpine(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = def.Close() })
	if err := def.Publish(ctx, infrastructure.Event{ObjectID: "OBJ-4", Kind: "goal", Op: "create"}); err != nil {
		t.Fatal(err)
	}

	missing := &WALSpine{path: filepath.Join(t.TempDir(), "nope.log")}
	if err := missing.Replay(ctx, 0, func(context.Context, infrastructure.Event) error { return nil }); err != nil {
		t.Fatal(err)
	}
	empty := &WALSpine{}
	if err := empty.Close(); err != nil {
		t.Fatal(err)
	}
}
