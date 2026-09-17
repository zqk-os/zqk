package agentprompt

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/workflow/whatsnext"
)

func TestCapStageTemplateCoverage_complete(t *testing.T) {
	if missing := CapStageTemplateCoverage(); len(missing) != 0 {
		t.Fatalf("CapStages missing template ids: %v", missing)
	}
	if len(CapStagePromptTemplateIDs) != len(whatsnext.CapStages) {
		t.Fatalf("map size %d != CapStages %d", len(CapStagePromptTemplateIDs), len(whatsnext.CapStages))
	}
}

func TestBuildCapStageWakePrompt_fallbackWithoutStorage(t *testing.T) {
	sec := pkgctx.NewSystemSecurityContext()
	got, err := BuildCapStageWakePrompt(context.Background(), nil, sec, "cap_stage_design", CapStageWakeOptions{
		PlanID: "PRI-test",
		CvsID:  "CVS-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CAP loop", "cap_stage_design", "PRI-test", "CVS-test", "cap_stage:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("fallback body missing %q:\n%s", want, got)
		}
	}
}

func TestBuildCapStageWakePrompt_requiresStage(t *testing.T) {
	_, err := BuildCapStageWakePrompt(context.Background(), nil, pkgctx.NewSystemSecurityContext(), "", CapStageWakeOptions{})
	if err == nil {
		t.Fatal("expected error for empty stage")
	}
}
