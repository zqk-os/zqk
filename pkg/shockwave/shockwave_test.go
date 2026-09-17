package shockwave

import (
	"context"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
)

// MockSplitter splits a payload into pieces
type MockSplitter struct{}

func (m *MockSplitter) Split(ctx context.Context, payload interface{}) ([]interface{}, error) {
	str := payload.(string)
	return []interface{}{str + "_1", str + "_2"}, nil
}

// MockDuplicator duplicates a payload
type MockDuplicator struct{}

func (m *MockDuplicator) Duplicate(ctx context.Context, payload interface{}) ([]interface{}, error) {
	return []interface{}{payload, payload}, nil
}

// MockHandler processes a payload
type MockHandler struct{}

func (m *MockHandler) Handle(ctx context.Context, payload interface{}, targetTiers TierMask) (interface{}, error) {
	suffix := "_processed"
	if targetTiers&TierSecurity != 0 {
		suffix += "_security"
	}
	return payload.(string) + suffix, nil
}

func TestNeuron_DispatchSplit(t *testing.T) {
	logger := logging.GetLoggerFromProfile("system")
	neuron := NewNeuron(PropagationRules{
		MaxDepth:    3,
		TargetTiers: TierSecurity | TierSyntax,
	}, logger)

	neuron.RegisterSplitter(&MockSplitter{})
	neuron.RegisterHandler(&MockHandler{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	neuron.Start(ctx)

	err := neuron.Send(ctx, "payload")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read results
	var results []interface{}
	timer := time.NewTimer(time.Second)

	for len(results) < 2 {
		select {
		case res := <-neuron.Receive():
			results = append(results, res)
		case <-timer.C:
			t.Fatalf("timeout waiting for results")
		}
	}
	neuron.Stop()

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// Because processing is async, the order might not be guaranteed
	// But let's check contents
	found1 := false
	found2 := false
	for _, r := range results {
		str := r.(string)
		if str == "payload_1_processed_security" {
			found1 = true
		}
		if str == "payload_2_processed_security" {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Errorf("unexpected results: %v", results)
	}
}

func TestNeuron_DispatchDuplicate(t *testing.T) {
	logger := logging.GetLoggerFromProfile("system")
	neuron := NewNeuron(PropagationRules{
		MaxDepth:    3,
		TargetTiers: TierSecurity | TierSyntax,
	}, logger)

	neuron.RegisterDuplicator(&MockDuplicator{})
	neuron.RegisterHandler(&MockHandler{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	neuron.Start(ctx)

	err := neuron.Send(ctx, "payload")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []interface{}
	timer := time.NewTimer(time.Second)

	for len(results) < 2 {
		select {
		case res := <-neuron.Receive():
			results = append(results, res)
		case <-timer.C:
			t.Fatalf("timeout waiting for results")
		}
	}
	neuron.Stop()

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	if results[0].(string) != "payload_processed_security" || results[1].(string) != "payload_processed_security" {
		t.Errorf("unexpected results: %v", results)
	}
}
