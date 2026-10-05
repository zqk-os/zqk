package cli

import (
	"context"
	"path/filepath"
	"testing"

	pkgcli "github.com/zqk-os/zqk/pkg/cli"
	clicontext "github.com/zqk-os/zqk/pkg/cliapp/context"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
)

func TestCommandContextOr(t *testing.T) {
	t.Parallel()
	fallback := pkgctx.NewSystemContext()
	if nilCmd := CommandContextOr(nil, fallback); nilCmd != fallback {
		t.Fatal("nil cmd should use fallback")
	}
	cmd := pkgcli.NewCommandBuilder("x").Build()
	if noCtx := CommandContextOr(cmd, fallback); noCtx != fallback {
		t.Fatal("cmd without context should use fallback")
	}
	stdCtx := context.Background()
	cmd.SetContext(stdCtx)
	if got := CommandContextOr(cmd, fallback); got != stdCtx {
		t.Fatal("cmd with context should use cmd.Context()")
	}
}

func TestContextFromInner(t *testing.T) {
	t.Parallel()
	inner := &clicontext.Context{ProjectRoot: "/tmp"}
	w := ContextFromInner(inner)
	if w == nil {
		t.Fatal("expected non-nil wrapper")
	}
	if w.Context != inner {
		t.Fatal("expected embedded inner context")
	}
	if w.Format != FormatTable {
		t.Fatalf("Format: got %q want table", w.Format)
	}
	if ContextFromInner(nil) != nil {
		t.Fatal("nil inner must yield nil wrapper")
	}
}

func TestWrapperContext_PathResolver(t *testing.T) {
	t.Parallel()
	w := ContextForProjectRoot("/tmp/wrap-root")
	paths.ReplacePathCache("/tmp/wrap-root", paths.DefaultPathAliases())
	r := w.PathResolver()
	if r.ProjectRoot() != "/tmp/wrap-root" {
		t.Fatalf("ProjectRoot: %q", r.ProjectRoot())
	}
	got, err := r.ResolveStrict(paths.PathSchemePrefix + "docs")
	if err != nil {
		t.Fatalf("ResolveStrict: %v", err)
	}
	want := filepath.Join("/tmp/wrap-root", paths.DocsDir)
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

type stubPathResolverWrapper struct{ id int }

func (s *stubPathResolverWrapper) ProjectRoot() string { return "/w" }

func (s *stubPathResolverWrapper) ResolveStrict(string) (string, error) { return "/w/p", nil }

func (s *stubPathResolverWrapper) ResolveFromCacheOrConstant(_, fb string) string { return "/w/" + fb }

func TestWrapperContext_WithPathResolver(t *testing.T) {
	t.Parallel()
	stub := &stubPathResolverWrapper{id: 1}
	w := ContextForProjectRoot("/tmp/x").WithPathResolver(stub)
	if w.PathResolver() != stub {
		t.Fatal("wrapper should return injected PathResolver")
	}
}
