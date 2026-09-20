package bldr_lifecycle_v1

import "testing"

func TestNewAgentSkillLifecycleBuilder(t *testing.T) {
	t.Parallel()
	lifecycle := NewAgentSkillLifecycleBuilder().Build()
	if lifecycle.ObjectType != "agent_skill" || len(lifecycle.Statuses) == 0 {
		t.Fatalf("unexpected agent_skill lifecycle: %#v", lifecycle)
	}
}
