package scheduler

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestSubprocessEnvForCVSOrchestrateRollupOnly_replacesKeys(t *testing.T) {
	pr := zqkenv.ProjectRoot().Name()
	sp := EnvKeyCVSOrchestrateSkipPersist
	t.Setenv(pr, "/wrong/root")
	t.Setenv(sp, "0")

	env := subprocessEnvForCVSOrchestrateRollupOnly("/correct/root", "")

	var prCount, spCount int
	for _, e := range env {
		switch {
		case strings.HasPrefix(e, pr+"="):
			prCount++
			if want := pr + "=/correct/root"; e != want {
				t.Fatalf("project root entry: got %q want %q", e, want)
			}
		case strings.HasPrefix(e, sp+"="):
			spCount++
			if want := sp + "=" + EnvValueCVSOrchestrateSkipPersist; e != want {
				t.Fatalf("skip persist entry: got %q want %q", e, want)
			}
		}
	}
	if prCount != 1 {
		t.Fatalf("expected exactly one %s entry, got %d", pr, prCount)
	}
	if spCount != 1 {
		t.Fatalf("expected exactly one %s entry, got %d", sp, spCount)
	}
}

func TestSubprocessEnvForCVSOrchestrateRollupOnly_setsRollupOutFromJob(t *testing.T) {
	t.Setenv(EnvKeyCVSOrchestrateRollupOut, "/stale/rollup.json")
	env := subprocessEnvForCVSOrchestrateRollupOnly("/correct/root", "/want/rollup.json")
	rollupPrefix := EnvKeyCVSOrchestrateRollupOut + "="
	var count int
	for _, e := range env {
		if strings.HasPrefix(e, rollupPrefix) {
			count++
			if want := rollupPrefix + "/want/rollup.json"; e != want {
				t.Fatalf("rollup out entry: got %q want %q", e, want)
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one %s entry, got %d", EnvKeyCVSOrchestrateRollupOut, count)
	}
}

func TestConvergenceOrchestrateScriptPath(t *testing.T) {
	t.Parallel()
	root := filepath.Join(string(filepath.Separator), "repo", "root")
	p := convergenceOrchestrateScriptPath(root)
	want := filepath.Join(root, paths.ScriptsDir, convergenceOrchestrateScriptFile)
	if p != want {
		t.Fatalf("got %q want %q", p, want)
	}
}

func TestNewCVSOrchestrateRollupOnlyCommand_fields(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "repo")
	sid := "[REDACTED-ID]"
	cmd := newCVSOrchestrateRollupOnlyCommand(ctx, root, sid, "")
	if cmd.Dir != root {
		t.Fatalf("Dir: got %q want %q", cmd.Dir, root)
	}
	wantScript := convergenceOrchestrateScriptPath(root)
	if len(cmd.Args) < 3 || cmd.Args[0] != wantScript || cmd.Args[1] != sid || cmd.Args[2] != convergenceOrchestrateArgNoFailOnGates {
		t.Fatalf("Args: %#v (want [script %s %s])", cmd.Args, sid, convergenceOrchestrateArgNoFailOnGates)
	}
	pr := zqkenv.ProjectRoot().Name() + "="
	sp := EnvKeyCVSOrchestrateSkipPersist + "="
	var sawPR, sawSkip bool
	for _, e := range cmd.Env {
		switch {
		case strings.HasPrefix(e, pr):
			if e != pr+root {
				t.Fatalf("env project root: %q", e)
			}
			sawPR = true
		case e == sp+EnvValueCVSOrchestrateSkipPersist:
			sawSkip = true
		}
	}
	if !sawPR || !sawSkip {
		t.Fatalf("missing env: pr=%v skip=%v", sawPR, sawSkip)
	}
}

func TestNewCVSOrchestrateRollupOnlyCommand_envRollupOut(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "repo")
	sid := "[REDACTED-ID]"
	wantOut := filepath.Join(root, "custom", "rollup.json")
	cmd := newCVSOrchestrateRollupOnlyCommand(ctx, root, sid, wantOut)
	rollupPrefix := EnvKeyCVSOrchestrateRollupOut + "="
	var saw bool
	for _, e := range cmd.Env {
		if e == rollupPrefix+wantOut {
			saw = true
			break
		}
	}
	if !saw {
		t.Fatalf("missing %s=%s in cmd.Env", EnvKeyCVSOrchestrateRollupOut, wantOut)
	}
}
