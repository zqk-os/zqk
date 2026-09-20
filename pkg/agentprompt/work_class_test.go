package agentprompt

import "testing"

func TestClassifyWorkClass(t *testing.T) {
	t.Parallel()

	cef := ClassifyWorkClass(
		"CEF AGENT_AD W1_arch_specialist — L-ARCHITECTURE",
		"pipeline_ref=PIP-CEF-DIAMOND-REMEASURE-001\nFORBID: source edits\ndocs/quality/cef-runs/2026-09-03-AGENT_AD",
	)
	if cef != WorkClassDocsEval {
		t.Fatalf("got %q want docs_eval", cef)
	}
	if ClassifyWorkClass("fix pkg/scheduler/swarm_worker.go") != WorkClassCoding {
		t.Fatal("coding ATK must stay coding")
	}
	if !WorkClassDocsEval.IsDocsEval() || WorkClassCoding.IsDocsEval() {
		t.Fatal("IsDocsEval mismatch")
	}
}
