// BLI-STARTER-COMMUNITY-028 / PRI-STARTER-COMMUNITY-028 coverage elevation
package ollama

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestAdapter_MatchAndID(t *testing.T) {
	t.Parallel()
	a := Adapter{}
	if a.ID() != id {
		t.Fatalf("ID=%q", a.ID())
	}
	if !a.Match("Ollama", "") {
		t.Fatal("provider match")
	}
	if !a.Match("", "http://127.0.0.1:11434/v1") {
		t.Fatal("port match")
	}
	if a.Match("openai", "https://api.openai.com/v1") {
		t.Fatal("openai should not match")
	}
}

func TestAdapter_MatchOLLAMAHost(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "http://10.0.0.9:21434")
	a := Adapter{}
	if !a.Match("", "http://10.0.0.9:21434/v1") {
		t.Fatal("host substring")
	}
}

func TestAdapter_DetectGuards(t *testing.T) {
	a := Adapter{}
	if a.Detect(nil) {
		t.Fatal("nil cfg")
	}
	if a.Detect(&llm.Config{BaseURL: defaultBaseURL}) {
		t.Fatal("already has URL")
	}
	t.Setenv(zqkenv.DisableLocalOllama().Name(), "1")
	if a.Detect(&llm.Config{}) {
		t.Fatal("disabled")
	}
}

func TestAdapter_FillDefaultsFromEnv(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "10.1.2.3")
	t.Setenv("OLLAMA_MODEL", "llama3")
	t.Setenv("OLLAMA_EMBED_MODEL", "embed-x")
	cfg := &llm.Config{}
	a := Adapter{}
	a.FillDefaults(cfg)
	if cfg.BaseURL != "http://10.1.2.3:11434/v1" && cfg.BaseURL != "http://10.1.2.3/v1" {
		t.Fatalf("BaseURL=%q", cfg.BaseURL)
	}
	if cfg.ChatModel != "llama3" || cfg.EmbedModel != "embed-x" {
		t.Fatalf("models %q %q", cfg.ChatModel, cfg.EmbedModel)
	}
	if cfg.Provider != id {
		t.Fatalf("provider %q", cfg.Provider)
	}
	a.FillDefaults(nil)
	already := &llm.Config{BaseURL: "http://127.0.0.1:11434"}
	a.FillDefaults(already)
	if already.BaseURL != "http://127.0.0.1:11434/v1" {
		t.Fatalf("ensureV1: %q", already.BaseURL)
	}
}

func TestClassifyModel_empty(t *testing.T) {
	t.Parallel()
	a := Adapter{}
	if _, ok := a.ClassifyModel(" "); ok {
		t.Fatal("blank")
	}
	if _, ok := a.ClassifyModel("llama3"); ok {
		t.Fatal("non-coder")
	}
}

func TestAdapter_DetectForceLocalProbesLoopback(t *testing.T) {
	t.Setenv(zqkenv.DisableLocalOllama().Name(), "")
	t.Setenv(zqkenv.ForceLocalOllama().Name(), "1")
	a := Adapter{}
	_ = a.Detect(&llm.Config{}) // true only if something is listening on 11434
	if a.Detect(&llm.Config{BaseURL: "http://127.0.0.1:11434/v1"}) {
		t.Fatal("explicit URL skips detect")
	}
}

func TestHelpers_localListeningAndFlags(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "127.0.0.1:9")
	_ = localListening()
	t.Setenv(zqkenv.ForceLocalOllama().Name(), "true")
	if !enabledLocal() {
		t.Fatal("force enable")
	}
	t.Setenv(zqkenv.DisableLocalOllama().Name(), "true")
	if !disabledLocal() {
		t.Fatal("disable")
	}
}

func TestNormalizeAndDialHelpers(t *testing.T) {
	t.Parallel()
	if normalizeBaseURL("") != defaultBaseURL {
		t.Fatal("empty host")
	}
	if got := ensureV1("http://x/v1/foo"); got != "http://x/v1/foo" {
		t.Fatalf("keep v1 path: %q", got)
	}
	if dialTarget("") != "" {
		t.Fatal("empty dial")
	}
	if got := dialTarget("http://127.0.0.1:11434"); got != "127.0.0.1:11434" {
		t.Fatalf("url host: %q", got)
	}
	if got := dialTarget("127.0.0.1"); got != "127.0.0.1:11434" {
		t.Fatalf("bare host: %q", got)
	}
}
