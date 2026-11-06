package nodesovereignty_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/nodesovereignty"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestAssess_okWhenAligned(t *testing.T) {
	root := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), root)
	rep := nodesovereignty.Assess(root)
	if rep.Severity != nodesovereignty.SeverityOK {
		t.Fatalf("%+v", rep)
	}
	if rep.RootID == "" || rep.SeatedRoot == "" {
		t.Fatal("missing ids")
	}
}

func TestAssess_mcpPinHijack(t *testing.T) {
	base := filepath.Join(fileutil.TempDir(), "zqk-nodesov-mcp-hijack")
	_ = fileutil.RemoveAll(base)
	t.Cleanup(func() { _ = fileutil.RemoveAll(base) })
	home := filepath.Join(base, "home")
	seated := filepath.Join(base, "seated")
	other := filepath.Join(base, "other")
	for _, d := range []string{home, seated, other} {
		if err := fileutil.MkdirAll(d, paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv(zqkenv.ProjectRoot().Name(), seated)
	cursor := filepath.Join(home, ".cursor")
	if err := fileutil.MkdirAll(cursor, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{
		"mcpServers": map[string]any{
			"zqk": map[string]any{
				"env": map[string]any{"ZQK_PROJECT_ROOT": other},
			},
		},
	}
	b, _ := json.Marshal(doc)
	if err := fileutil.WriteFile(filepath.Join(cursor, "mcp.json"), b, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	rep := nodesovereignty.Assess(seated)
	if rep.Severity != nodesovereignty.SeverityHijacked {
		t.Fatalf("want hijacked got %+v", rep)
	}
	if rep.ATTN == "" {
		t.Fatal("expected ATTN")
	}
}

func TestRecordHopAndRefuse(t *testing.T) {
	from := t.TempDir()
	to := t.TempDir()
	// Feed append needs chat channel; skip feed failure by ensuring dirs exist
	if err := fileutil.MkdirAll(filepath.Join(to, paths.ProjectDataDir), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(zqkenv.ProjectRoot().Name(), from)
	if err := nodesovereignty.RefuseCrossRootMutate(to); err == nil {
		t.Fatal("expected refuse without hop")
	}
	if err := nodesovereignty.RecordHop(from, to, "test", "unit"); err != nil {
		// AppendEvent may fail without lite feed — hop state should still land
		t.Logf("RecordHop: %v", err)
	}
	// Force-write hop state if feed blocked RecordHop early — RecordHop writes state before feed
	if !nodesovereignty.HasRecentHop(to, time.Hour) {
		// Retry after ensuring feed config exists is heavy; write state via second RecordHop
		// with Skip — instead recreate minimal state
		_ = fileutil.MkdirAll(filepath.Join(to, paths.ProjectDataDir, paths.StateDir, "node_sovereignty"), paths.DirPerm755)
	}
	if err := nodesovereignty.RecordHop(from, to, "test", "unit"); err != nil {
		t.Log(err)
	}
	if !nodesovereignty.HasRecentHop(to, time.Hour) {
		t.Fatal("expected hop record")
	}
	if err := nodesovereignty.RefuseCrossRootMutate(to); err != nil {
		t.Fatalf("hop should allow: %v", err)
	}
}
