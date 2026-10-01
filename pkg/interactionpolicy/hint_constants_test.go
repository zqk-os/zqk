package interactionpolicy

import "testing"

// File-local named constants for test identifiers and message templates,
// satisfying the no-magic-string structural rule.
const (
	testNameAlignRefresh  = "align_refresh"
	testNameDraftClassify = "draft_classify"
	testNameHourglass     = "hourglass"
	testNameTracePipe     = "trace_pipeline"
	testNameOrchPrefix    = "orch_prefix"
	testNameOrchSuffix    = "orch_suffix"
	testNameObjPrefix     = "object_prefix"
	testNameObjSuffix     = "object_suffix"
	testNameSwarmInit     = "swarm_init"

	msgNotEmpty    = "hint constant %q must not be empty"
	msgDuplicate   = "hint constants %q and %q both = %q (must be distinct)"
	msgKnownFalse  = "IsKnownHint(%q) = false, want true"
	msgUnknownTrue = "IsKnownHint(%q) = true, want false"
	msgEmptyTrue   = "IsKnownHint(empty) = true, want false"
	unknownProbe   = "totally unrelated prose"
)

func TestHintConstants_NonEmptyDistinct(t *testing.T) {
	entries := []struct {
		name string
		lit  string
	}{
		{testNameAlignRefresh, HintAlignRefreshText},
		{testNameDraftClassify, HintDraftClassifyText},
		{testNameHourglass, HintHourglassText},
		{testNameTracePipe, HintTracePipelineText},
		{testNameOrchPrefix, HintOrchestratePrefix},
		{testNameOrchSuffix, HintOrchestrateSuffix},
		{testNameObjPrefix, HintObjectGetPrefix},
		{testNameObjSuffix, HintObjectGetSuffix},
		{testNameSwarmInit, HintSwarmInitText},
	}
	seen := map[string]string{}
	for _, e := range entries {
		if e.lit == "" {
			t.Fatalf(msgNotEmpty, e.name)
		}
		if prev, dup := seen[e.lit]; dup {
			t.Fatalf(msgDuplicate, prev, e.name, e.lit)
		}
		seen[e.lit] = e.name
	}
}

func TestIsKnownHint(t *testing.T) {
	for _, lit := range []string{
		HintAlignRefreshText,
		HintDraftClassifyText,
		HintHourglassText,
		HintTracePipelineText,
	} {
		if !IsKnownHint(lit) {
			t.Fatalf(msgKnownFalse, lit)
		}
	}
	if IsKnownHint(unknownProbe) {
		t.Fatalf(msgUnknownTrue, unknownProbe)
	}
	if IsKnownHint("") {
		t.Fatal(msgEmptyTrue)
	}
}
