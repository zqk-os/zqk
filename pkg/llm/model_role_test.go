package llm

import "testing"

func TestClassifyChatModel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want ModelRole
	}{
		{in: "qwen3.6:latest", want: ModelRoleAgentDoer},
		{in: "Qwen/Qwen2.5-7B-Instruct", want: ModelRoleAgentDoer},
		{in: "gpt-4o-mini", want: ModelRoleAgentDoer},
		{in: "qwen2.5-coder:7b", want: ModelRoleCodeDraft},
		{in: "Qwen/Qwen2.5-Coder-7B-Instruct", want: ModelRoleCodeDraft},
		{in: "qwen2.5-coder:32b", want: ModelRoleCodeDraft},
		{in: "deepseek-coder:6.7b", want: ModelRoleCodeDraft},
		{in: "", want: ModelRoleAgentDoer},
	}
	for _, tc := range cases {
		if got := ClassifyChatModel(tc.in); got != tc.want {
			t.Fatalf("ClassifyChatModel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if !IsCodeDraftModel("qwen2.5-coder:7b") {
		t.Fatal("expected code-draft")
	}
	if IsCodeDraftModel("qwen3.6:latest") {
		t.Fatal("qwen3.6 is a doer")
	}
}
