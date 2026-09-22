// BLI-STARTER-COMMUNITY-028 / PRI-STARTER-COMMUNITY-028 coverage elevation
package qwen

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/llm"
)

func TestAdapter_MatchFillClassify(t *testing.T) {
	t.Parallel()
	a := Adapter{}
	if a.ID() != id {
		t.Fatalf("ID=%q", a.ID())
	}
	if !a.Match("Qwen", "") {
		t.Fatal("provider")
	}
	if !a.Match("", "https://dashscope.aliyuncs.com/compatible-mode/v1") {
		t.Fatal("dashscope")
	}
	if a.Match("openai", "https://api.openai.com/v1") {
		t.Fatal("openai host")
	}

	local := &llm.Config{}
	a.FillDefaults(local)
	if local.BaseURL != localURL || local.ChatModel != localChat || local.APIKey != dummyAPIKey {
		t.Fatalf("local %+v", local)
	}

	cloud := &llm.Config{BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1"}
	a.FillDefaults(cloud)
	if cloud.ChatModel != cloudChat || cloud.EmbedModel != embedModel {
		t.Fatalf("cloud %+v", cloud)
	}

	rewrite := &llm.Config{BaseURL: "https://api.openai.com/v1", ChatModel: "gpt-4o-mini", EmbedModel: "text-embedding-3-small"}
	a.FillDefaults(rewrite)
	if rewrite.BaseURL != localURL || rewrite.ChatModel != localChat {
		t.Fatalf("rewrite %+v", rewrite)
	}

	a.FillDefaults(nil)
	if _, ok := a.ClassifyModel(""); ok {
		t.Fatal("empty")
	}
	role, ok := a.ClassifyModel("Qwen2.5-Coder-7B")
	if !ok || role != llm.ModelRoleCodeDraft {
		t.Fatalf("coder %q %v", role, ok)
	}
	if _, ok := a.ClassifyModel("qwen-max"); ok {
		t.Fatal("non-coder")
	}
}
