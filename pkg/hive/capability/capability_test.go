package capability_test

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/hive/capability"
	"github.com/stretchr/testify/assert"
)

type mockCapability struct {
	name        string
	description string
	allowed     bool
	err         error
}

func (m *mockCapability) Name() string        { return m.name }
func (m *mockCapability) Description() string { return m.description }
func (m *mockCapability) EvaluatePolicy(ctx context.Context, evalCtx capability.EvalContext) (bool, error) {
	return m.allowed, m.err
}

func TestRegistry(t *testing.T) {
	reg := capability.NewRegistry()

	cap1 := &mockCapability{name: "fs:read", description: "Read files", allowed: true}

	// Test Register
	if err := reg.Register(cap1); err != nil {
		t.Fatalf("Failed to register capability: %v", err)
	}

	// Test Duplicate Register
	if err := reg.Register(cap1); err != capability.ErrCapabilityExists {
		t.Errorf("Expected ErrCapabilityExists, got: %v", err)
	}

	// Test Get
	c, err := reg.Get("fs:read")
	if err != nil {
		t.Fatalf("Failed to get capability: %v", err)
	}
	if c.Name() != "fs:read" {
		t.Errorf("Expected fs:read, got %v", c.Name())
	}

	// Test Get Non-Existent
	_, err = reg.Get("fs:write")
	if err != capability.ErrCapabilityNotFound {
		t.Errorf("Expected ErrCapabilityNotFound, got: %v", err)
	}

	// Test List
	list := reg.List()
	if len(list) != 1 {
		t.Errorf("Expected 1 capability in list, got %d", len(list))
	}
}

func TestEvaluatePolicy(t *testing.T) {
	ctx := context.Background()
	capAllowed := &mockCapability{name: "allowed", allowed: true}
	capDenied := &mockCapability{name: "denied", allowed: false}

	allowed, err := capAllowed.EvaluatePolicy(ctx, capability.EvalContext{})
	assert.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = capDenied.EvaluatePolicy(ctx, capability.EvalContext{})
	assert.NoError(t, err)
	assert.False(t, allowed)
}

func TestSynthesisPipeline(t *testing.T) {
	reg := capability.NewRegistry()
	pipeline := capability.NewSynthesisPipeline(reg)
	capItem := &mockCapability{name: "synth:test", description: "Test synthesis capability"}

	// Must fail when testCaseRefs is empty, nil, or contains only whitespace
	assert.Equal(t, capability.ErrTestCaseRequired, pipeline.Synthesize(capItem, nil))
	assert.Equal(t, capability.ErrTestCaseRequired, pipeline.Synthesize(capItem, []string{}))
	assert.Equal(t, capability.ErrTestCaseRequired, pipeline.Synthesize(capItem, []string{""}))
	assert.Equal(t, capability.ErrTestCaseRequired, pipeline.Synthesize(capItem, []string{"   "}))

	// Must succeed when testCaseRefs contains at least 1 valid test_case
	err := pipeline.Synthesize(capItem, []string{"TC-101"})
	assert.NoError(t, err)

	fetched, err := reg.Get("synth:test")
	assert.NoError(t, err)
	assert.Equal(t, "synth:test", fetched.Name())
}
