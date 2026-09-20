package workflow

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestBuildWhatsNextCorrespondence_requiresAgentID(t *testing.T) {
	corr := buildWhatsNextCorrespondence(t.TempDir(), "", "PER-1", []string{"PER-1"}, nil)
	if corr == nil || corr.SkipReason != "agent_id_required" {
		t.Fatalf("corr=%+v", corr)
	}
}

func TestBuildWhatsNextCorrespondence_withActiveTask(t *testing.T) {
	corr := buildWhatsNextCorrespondence(t.TempDir(), "peer-agent-01", "PER-1", nil, map[string]any{
		objects.FieldKeyID:     "ATK-1",
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		objects.FieldKeyTitle:  "Do thing",
	})
	if corr.ActiveAgentTask == nil || corr.ActiveAgentTask.ID != "ATK-1" {
		t.Fatalf("active=%+v", corr.ActiveAgentTask)
	}
	if corr.NextActionHint != agentfeed.HintStandbyForbidden && corr.SkipReason == "" {
		// empty feed → continue upgraded to standby_forbidden when task active
		if corr.NextActionHint != agentfeed.HintStandbyForbidden {
			t.Fatalf("hint=%q skip=%q", corr.NextActionHint, corr.SkipReason)
		}
	}
}
