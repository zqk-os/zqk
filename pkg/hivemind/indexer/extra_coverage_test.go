// BLI-STARTER-COMMUNITY-044 / PRI-STARTER-COMMUNITY-044 coverage elevation
package indexer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/lifecycle"
)

type extraFailEmbed struct{}

func (extraFailEmbed) Embed(context.Context, string) ([]float32, error) {
	return nil, errors.New("embed")
}

type extraFailStore struct{}

func (extraFailStore) Upsert(context.Context, string, []float32) error {
	return errors.New("upsert")
}

func TestExtraIndexerWorkerAndListenerSkips(t *testing.T) {
	ctx := context.Background()
	if err := NewIndexerWorker(extraFailEmbed{}, &MockStore{}).Index(ctx, "id", "t"); err == nil {
		t.Fatal("embed err")
	}
	if err := NewIndexerWorker(&MockEmbedding{}, extraFailStore{}).Index(ctx, "id", "t"); err == nil {
		t.Fatal("upsert err")
	}

	tmp := t.TempDir()
	wal, err := lifecycle.NewLifecycleEventWAL(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if err := wal.Append(&lifecycle.LifecycleEvent{EventType: "other", Kind: "requirement", ID: "r-skip"}); err != nil {
		t.Fatal(err)
	}
	if err := wal.Append(&lifecycle.LifecycleEvent{EventType: lifecycle.EventTypeStatusTransition, Kind: "backlog_item", ID: "bli-1"}); err != nil {
		t.Fatal(err)
	}
	if err := wal.Append(&lifecycle.LifecycleEvent{EventType: lifecycle.EventTypeStatusTransition, Kind: "specification", ID: "spec-1"}); err != nil {
		t.Fatal(err)
	}
	if err := wal.Sync(); err != nil {
		t.Fatal(err)
	}

	listener := NewIndexerListener(wal, NewIndexerWorker(extraFailEmbed{}, &MockStore{}))
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- listener.Run(runCtx) }()
	time.Sleep(150 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not stop")
	}
}
