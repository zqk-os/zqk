package bldr_lifecycle_v1

import "testing"

func TestNewDivisionLifecycleBuilder(t *testing.T) {
	t.Parallel()
	lifecycle := NewDivisionLifecycleBuilder().Build()
	if lifecycle.ObjectType != "division" || len(lifecycle.Statuses) == 0 {
		t.Fatalf("unexpected division lifecycle: %#v", lifecycle)
	}
}
