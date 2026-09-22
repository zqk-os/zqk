// BLI-STARTER-COMMUNITY-035 / PRI-STARTER-COMMUNITY-035 coverage elevation
package shockwave

import (
	"context"
	"errors"
	"testing"
	"time"
)

type errSplitter struct{}

func (errSplitter) Split(context.Context, interface{}) ([]interface{}, error) {
	return nil, errors.New("split")
}

type errHandler struct{}

func (errHandler) Handle(context.Context, interface{}, TierMask) (interface{}, error) {
	return nil, errors.New("handle")
}

type passthroughHandler struct{}

func (passthroughHandler) Handle(_ context.Context, payload interface{}, _ TierMask) (interface{}, error) {
	return payload, nil
}

type errDuplicator struct{}

func (errDuplicator) Duplicate(context.Context, interface{}) ([]interface{}, error) {
	return nil, errors.New("dup")
}

func TestNeuron_NilLoggerErrorsAndForward(t *testing.T) {
	n := NewNeuron(PropagationRules{MaxWorkers: 2}, nil)
	n.RegisterSplitter(errSplitter{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	n.Start(ctx)
	if err := n.Send(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	n.Stop()

	n2 := NewNeuron(PropagationRules{MaxWorkers: 2}, nil)
	n2.RegisterHandler(errHandler{})
	n2.RegisterHandler(passthroughHandler{})
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	n2.Start(ctx2)
	if err := n2.Send(ctx2, "y"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-n2.Receive():
	case <-time.After(time.Second):
		t.Fatal("no forward")
	}
	n2.Stop()

	n3 := NewNeuron(PropagationRules{MaxWorkers: 2}, nil)
	ctx3, cancel3 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel3()
	n3.Start(ctx3)
	if err := n3.Send(ctx3, "z"); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-n3.Receive():
		if got != "z" {
			t.Fatalf("%v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("no passthrough")
	}
	n3.Stop()

	n4 := NewNeuron(PropagationRules{MaxWorkers: 2}, nil)
	n4.RegisterDuplicator(errDuplicator{})
	ctx4, cancel4 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel4()
	n4.Start(ctx4)
	if err := n4.Send(ctx4, "d"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	n4.Stop()
}
