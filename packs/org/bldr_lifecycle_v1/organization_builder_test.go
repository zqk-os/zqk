package bldr_lifecycle_v1

import "testing"

func TestNewOrganizationLifecycleBuilder(t *testing.T) {
	t.Parallel()
	lifecycle := NewOrganizationLifecycleBuilder().Build()
	if lifecycle.ObjectType != "organization" || len(lifecycle.Statuses) == 0 {
		t.Fatalf("unexpected organization lifecycle: %#v", lifecycle)
	}
}
