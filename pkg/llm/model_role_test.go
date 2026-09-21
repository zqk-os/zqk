package llm

import "testing"

func TestClassifyChatModel_coreHasNoVendorNames(t *testing.T) {
	t.Parallel()
	if got := ClassifyChatModel(""); got != ModelRoleAgentDoer {
		t.Fatalf("empty = %q", got)
	}
	if got := ClassifyChatModel("any-instruct"); got != ModelRoleAgentDoer {
		t.Fatalf("unknown = %q", got)
	}
	if !IsCodeDraftModel("coder:7b") {
		t.Fatal("small coder tag is a capability heuristic")
	}
	if IsCodeDraftModel("qwen3.6:latest") {
		t.Fatal("without a vendor adapter, qwen3.6 is not special-cased")
	}
}
