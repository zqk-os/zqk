package bldr_lifecycle_v1

import "testing"

func TestNewPersonaLifecycleBuilder(t *testing.T) {
	t.Parallel()
	lifecycle := NewPersonaLifecycleBuilder().Build()
	if lifecycle.ObjectType != "persona" || len(lifecycle.Statuses) == 0 {
		t.Fatalf("unexpected persona lifecycle: %#v", lifecycle)
	}
}
