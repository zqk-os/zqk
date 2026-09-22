// BLI-STARTER-COMMUNITY-046 / PRI-STARTER-COMMUNITY-046 coverage elevation
package infrastructure

import (
	"context"
	"errors"
	"testing"
	"time"
)

type extraSpine struct {
	pub    int
	closed bool
	subErr error
}

func (s *extraSpine) Publish(context.Context, Event) error { s.pub++; return nil }
func (s *extraSpine) Subscribe(context.Context, string, Handler) error {
	return s.subErr
}
func (s *extraSpine) Replay(context.Context, int64, Handler) error { return nil }
func (s *extraSpine) Close() error                                 { s.closed = true; return nil }

func TestExtraRegistryShadowAndBufferedSpine(t *testing.T) {
	ctx := context.Background()
	r := &Registry{spines: map[string]SpinalSpine{}, drivers: map[string]DriverFactory{}}
	if _, err := r.Engage(ctx, "a", "missing", "", ""); err == nil {
		t.Fatal("missing driver")
	}
	if _, err := r.GetSpine("a"); err == nil {
		t.Fatal("missing spine")
	}
	inner := &extraSpine{}
	r.RegisterDriver("stub", func(context.Context, string, string) (SpinalSpine, error) {
		return inner, nil
	})
	r.RegisterDriver("boom", func(context.Context, string, string) (SpinalSpine, error) {
		return nil, errors.New("engage fail")
	})
	if _, err := r.Engage(ctx, "b", "boom", "", ""); err == nil {
		t.Fatal("factory err")
	}
	got, err := r.Engage(ctx, "stub-1", "stub", "ep", "c")
	if err != nil || got == nil {
		t.Fatal(err)
	}
	again, err := r.Engage(ctx, "stub-1", "stub", "ep", "c")
	if err != nil || again != got {
		t.Fatal("cached engage")
	}
	if _, err := r.GetSpine("stub-1"); err != nil {
		t.Fatal(err)
	}

	shadow := NewShadowSpine(inner)
	if err := shadow.Publish(ctx, Event{ObjectID: "OBJ-1", Kind: "k", Op: "op"}); err != nil {
		t.Fatal(err)
	}
	if len(shadow.GetShadowEvents()) != 1 {
		t.Fatal("shadow")
	}
	if err := shadow.Subscribe(ctx, "k", nil); err != nil {
		t.Fatal(err)
	}
	if err := shadow.Replay(ctx, 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := shadow.Close(); err != nil || !inner.closed {
		t.Fatal("shadow close")
	}

	inner2 := &extraSpine{}
	buf := NewBufferedSpine(inner2, 4)
	t.Cleanup(func() { _ = buf.Close() })
	if err := buf.Publish(ctx, Event{ObjectID: "OBJ-2"}); err != nil {
		t.Fatal(err)
	}
	if err := buf.Subscribe(ctx, "k", nil); err != nil {
		t.Fatal(err)
	}
	if err := buf.Replay(ctx, 0, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	inner3 := &extraSpine{}
	tiny := NewBufferedSpine(inner3, 1)
	_ = tiny.Publish(ctx, Event{ObjectID: "fill"})
	_ = tiny.Publish(ctx, Event{ObjectID: "drop"})
	if err := tiny.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExtraGetRegistry(t *testing.T) {
	if GetRegistry() == nil || GetRegistry() != GetRegistry() {
		t.Fatal("singleton")
	}
}
