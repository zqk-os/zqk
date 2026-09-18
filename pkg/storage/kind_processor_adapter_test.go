package storage

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

type dummyKindAdapter struct {
	kind       string
	invocations int
}

func (d *dummyKindAdapter) Kind() string {
	return d.kind
}

func (d *dummyKindAdapter) Process(ctx context.Context, kctx *KindProcessorContext, obj map[string]any) error {
	d.invocations++
	return nil
}

func TestKindProcessorRegistry_RegistrationAndLookup(t *testing.T) {
	t.Parallel()
	r := NewKindProcessorRegistry()

	// Default adapters must be registered
	if r.Get(objects.KindAgentTask) == nil {
		t.Fatal("expected AgentTask adapter to be registered by default")
	}
	if r.Get(objects.KindPriorityPlan) == nil {
		t.Fatal("expected PriorityPlan adapter to be registered by default")
	}

	// Unregistered kind returns nil without error
	if r.Get("some_unknown_kind") != nil {
		t.Fatal("expected unregistered kind to return nil")
	}

	// Register custom adapter
	dummy := &dummyKindAdapter{kind: "custom_kind"}
	r.Register(dummy)
	if r.Get("custom_kind") != dummy {
		t.Fatal("expected custom_kind adapter to be retrieved")
	}

	// Process unregistered kind -> non-interrupting (returns nil)
	ctx := context.Background()
	kctx := &KindProcessorContext{}
	if err := r.Process(ctx, kctx, "unregistered_kind", map[string]any{}); err != nil {
		t.Fatalf("expected nil error for unregistered kind, got %v", err)
	}

	// Process registered kind
	if err := r.Process(ctx, kctx, "custom_kind", map[string]any{}); err != nil {
		t.Fatalf("expected nil error for custom_kind, got %v", err)
	}
	if dummy.invocations != 1 {
		t.Fatalf("expected 1 invocation, got %d", dummy.invocations)
	}
}

func TestKindProcessorRegistry_StorageIntegration(t *testing.T) {
	testRoot := t.TempDir()
	fos, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Core flow allows ordinary objects without adapters to pass through cleanly
	req := map[string]any{
		objects.FieldKeyID:          "REQ-TEST-ADAPTER-001",
		objects.FieldKeyKind:        objects.KindRequirement,
		objects.FieldKeyTitle:       "Test Requirement",
		objects.FieldKeyDescription: "Test Description for Requirement",
		objects.FieldKeyStatus:      objects.ObjectStatusConceptual,
	}
	if err := fos.Create(ctx, secCtx, req); err != nil {
		t.Fatalf("Create requirement without custom adapter should succeed, got: %v", err)
	}

	// 2. Custom adapter can be plugged in to enforce domain invariants on any kind
	customAdapter := &mockBlockingAdapter{kind: objects.KindRequirement}
	fos.GetKindProcessors().Register(customAdapter)

	req2 := map[string]any{
		objects.FieldKeyID:          "REQ-TEST-ADAPTER-002",
		objects.FieldKeyKind:        objects.KindRequirement,
		objects.FieldKeyTitle:       "Test Requirement 2",
		objects.FieldKeyDescription: "Test Description for Requirement 2",
		objects.FieldKeyStatus:      objects.ObjectStatusConceptual,
	}
	err = fos.Create(ctx, secCtx, req2)
	if err == nil {
		t.Fatal("expected registered adapter to intercept and fail, got nil")
	}
	if !strings.Contains(err.Error(), "Security Gate: mock adapter rejection") {
		t.Fatalf("expected 'Security Gate: mock adapter rejection', got: %v", err)
	}
}

type mockBlockingAdapter struct {
	kind string
}

func (m *mockBlockingAdapter) Kind() string {
	return m.kind
}

func (m *mockBlockingAdapter) Process(ctx context.Context, kctx *KindProcessorContext, obj map[string]any) error {
	return &testCustomError{msg: "Security Gate: mock adapter rejection"}
}

type testCustomError struct {
	msg string
}

func (e *testCustomError) Error() string {
	return e.msg
}
