package paths

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
)

func TestCLIUsage_usesLiveExecutable(t *testing.T) {
	prev := brand.ExecutableName()
	t.Cleanup(func() { brand.SetExecutableName(prev) })
	brand.SetExecutableName("acme-cli")
	got := CLIUsage("agent", "sync-loop", "<task-id>")
	if got != "acme-cli agent sync-loop <task-id>" {
		t.Fatalf("CLIUsage = %q", got)
	}
}

func TestCLIInvocation_rebrandsProductCommandsOnly(t *testing.T) {
	prev := brand.ExecutableName()
	t.Cleanup(func() { brand.SetExecutableName(prev) })
	brand.SetExecutableName("acme-cli")
	if got := CLIInvocation("zqk agent recover PRI-1"); got != "acme-cli agent recover PRI-1" {
		t.Fatalf("product = %q", got)
	}
	if got := CLIInvocation("git worktree remove /tmp/x"); got != "git worktree remove /tmp/x" {
		t.Fatalf("git = %q", got)
	}
	if got := CLIInvocation("object list backlog_item"); got != "acme-cli object list backlog_item" {
		t.Fatalf("suffix = %q", got)
	}
	got := CLIInvocation("zqk new object requirement && zqk workflow gen-trace-pipeline REQ-1")
	if got != "acme-cli new object requirement && acme-cli workflow gen-trace-pipeline REQ-1" {
		t.Fatalf("compound = %q", got)
	}
}

func TestRewriteCanonicalCLIInvocations_inProse(t *testing.T) {
	prev := brand.ExecutableName()
	t.Cleanup(func() { brand.SetExecutableName(prev) })
	brand.SetExecutableName("acme-cli")
	in := "Run zqk object update then git commit. Ignore the word zqk alone."
	got := RewriteCanonicalCLIInvocations(in)
	if got != "Run acme-cli object update then git commit. Ignore the word zqk alone." {
		t.Fatalf("got %q", got)
	}
}
