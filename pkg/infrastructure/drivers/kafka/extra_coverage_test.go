// BLI-STARTER-COMMUNITY-034 / PRI-STARTER-COMMUNITY-034 coverage elevation
package kafka

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/infrastructure"
)

func TestKafkaSpine_StubLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	spine, err := NewKafkaSpine(ctx, "localhost:9092", "creds")
	if err != nil {
		t.Fatal(err)
	}
	ev := infrastructure.Event{ObjectID: "O-1", Kind: "backlog_item", Op: "create"}
	if err := spine.Publish(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if err := spine.Subscribe(ctx, "backlog_item", func(context.Context, infrastructure.Event) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := spine.Replay(ctx, 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := spine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestHardenedKafkaSpine_CloseThenPublish(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	spine, err := NewHardenedKafkaSpine(ctx, "localhost:9092", "")
	if err != nil {
		t.Fatal(err)
	}
	ev := infrastructure.Event{ObjectID: "O-2", Kind: "priority_plan", Op: "update"}
	if err := spine.Publish(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if err := spine.Subscribe(ctx, ev.Kind, nil); err != nil {
		t.Fatal(err)
	}
	if err := spine.Replay(ctx, 3, nil); err != nil {
		t.Fatal(err)
	}
	if err := spine.Close(); err != nil {
		t.Fatal(err)
	}
	if err := spine.Publish(ctx, ev); err == nil {
		t.Fatal("closed publish")
	}
}
