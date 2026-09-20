package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentprompt"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSeatWorkerMustCallAssemblePreparedContext(t *testing.T) {
	t.Parallel()

	src, err := fileutil.ReadFile(filepath.Join("seat_worker.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if !strings.Contains(body, "AssemblePreparedContext") {
		t.Fatal("seat-worker must call AssemblePreparedContext; do not rebuild a thinner prompt")
	}
	if !strings.Contains(body, "clipSeatWorkerPromptPart") {
		t.Fatal("seat-worker must clip steer/prepared context at the 7B attach point")
	}
	for _, banned := range []string{
		"falling back to steer text",
		"using swarm templates",
	} {
		if strings.Contains(body, banned) {
			t.Fatalf("silent cold-start fallback reintroduced: %q", banned)
		}
	}
}

func TestClipSeatWorkerPromptPart_capsAndSteersWrite(t *testing.T) {
	t.Parallel()

	short := "steer only"
	if got := clipSeatWorkerPromptPart(short, seatWorkerSteerCap, "STEER", agentprompt.WorkClassCoding); got != short {
		t.Fatalf("short passthrough = %q", got)
	}

	dump := strings.Repeat("x", seatWorkerPreparedPromptCap+2000)
	got := clipSeatWorkerPromptPart(dump, seatWorkerPreparedPromptCap, "PREPARED CONTEXT", agentprompt.WorkClassCoding)
	if len(got) >= len(dump) {
		t.Fatalf("clipped len %d, original %d", len(got), len(dump))
	}
	if !strings.Contains(got, "TRUNCATED FOR WINDOW") {
		t.Fatalf("missing truncation footer: %q", got[len(got)-160:])
	}
	if !strings.Contains(got, "write_code") || strings.Contains(got, "cmd/") || strings.Contains(got, "pkg/") {
		t.Fatalf("coding clip footer must steer write_code without assuming a cmd/pkg layout: %q", got[len(got)-280:])
	}
	docs := clipSeatWorkerPromptPart(dump, seatWorkerPreparedPromptCap, "PREPARED CONTEXT", agentprompt.WorkClassDocsEval)
	if !strings.Contains(docs, "write_file") || !strings.Contains(docs, "application source") {
		t.Fatalf("docs_eval clip footer must steer write_file and spare application source: %q", docs[len(docs)-280:])
	}
	for _, studio := range []string{"docs/quality/", "cef-runs", "cmd/", "pkg/", "backlog"} {
		if strings.Contains(docs, studio) || strings.Contains(got, studio) {
			t.Fatalf("clip footer must not assume studio path %q: coding=%q docs=%q", studio, got[len(got)-200:], docs[len(docs)-200:])
		}
	}
	if strings.Contains(got, strings.Repeat("x", seatWorkerPreparedPromptCap+1)) {
		t.Fatal("clip kept more than the prepared-context cap")
	}
}

func TestIsPreparedContextRefusal(t *testing.T) {
	t.Parallel()

	if !isPreparedContextRefusal(errPreparedContextRequired) {
		t.Fatal("required-context must refuse")
	}
	if !isPreparedContextRefusal(errPreparedContextStorage) {
		t.Fatal("nil storage must refuse")
	}
	if isPreparedContextRefusal(nil) {
		t.Fatal("nil is not a refusal")
	}
}

func TestAssemblePreparedContext_WorkClassClassification(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:            "agent_prepared_ctx",
		SeedSchemaPlane: true,
	})
	root, fs := proj.Root, proj.FileStorage
	sec := pkgctx.NewSystemSecurityContext()

	// 1. Coding task with description
	codingCtx, err := AssemblePreparedContext(context.Background(), sec, fs, PreparedContextInput{
		Description: "implement pkg/storage feature with unit tests",
		ProjectRoot: root,
		IncludeTDD:  true,
	})
	if err != nil {
		t.Fatalf("AssemblePreparedContext coding: %v", err)
	}
	if codingCtx.WorkClass != agentprompt.WorkClassCoding {
		t.Fatalf("expected coding workClass, got %s", codingCtx.WorkClass)
	}
	if codingCtx.ExecRoot != "" {
		t.Fatalf("expected empty ExecRoot for coding, got %s", codingCtx.ExecRoot)
	}

	// 2. DocsEval task with description
	docsCtx, err := AssemblePreparedContext(context.Background(), sec, fs, PreparedContextInput{
		Description: "evaluate docs/quality/cef-runs/2026-09-04-AGENT_AD finding.schema.json",
		ProjectRoot: root,
		IncludeTDD:  true,
	})
	if err != nil {
		t.Fatalf("AssemblePreparedContext docs_eval: %v", err)
	}
	if docsCtx.WorkClass != agentprompt.WorkClassDocsEval {
		t.Fatalf("expected docs_eval workClass, got %s", docsCtx.WorkClass)
	}
	if docsCtx.ExecRoot != root {
		t.Fatalf("expected ExecRoot=%s for docs_eval, got %s", root, docsCtx.ExecRoot)
	}

	// 3. Explicit WorkClass override
	overrideCtx, err := AssemblePreparedContext(context.Background(), sec, fs, PreparedContextInput{
		Description: "plain description without keywords",
		WorkClass:   agentprompt.WorkClassDocsEval,
		ProjectRoot: root,
	})
	if err != nil {
		t.Fatalf("AssemblePreparedContext override: %v", err)
	}
	if overrideCtx.WorkClass != agentprompt.WorkClassDocsEval {
		t.Fatalf("expected overridden docs_eval workClass, got %s", overrideCtx.WorkClass)
	}
	if overrideCtx.ExecRoot != root {
		t.Fatalf("expected ExecRoot=%s for overridden docs_eval, got %s", root, overrideCtx.ExecRoot)
	}
}
