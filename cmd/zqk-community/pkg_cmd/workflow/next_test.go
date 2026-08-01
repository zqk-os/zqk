package workflow

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/policyinterrupt"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestTruncateWorkflowText(t *testing.T) {
	t.Parallel()
	if got := truncateWorkflowText("hello", 10); got != "hello" {
		t.Fatalf("got %q", got)
	}
	long := strings.Repeat("a", 100)
	got := truncateWorkflowText(long, 5)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis: %q", got)
	}
}

func TestStringSliceFromAnyField(t *testing.T) {
	t.Parallel()
	if s := stringSliceFromAnyField([]string{"a", "", " b "}); len(s) != 2 || s[0] != "a" || s[1] != "b" {
		t.Fatalf("got %#v", s)
	}
	if s := stringSliceFromAnyField([]any{"x", 1, "y"}); len(s) != 2 || s[0] != "x" || s[1] != "y" {
		t.Fatalf("got %#v", s)
	}
	if s := stringSliceFromAnyField(nil); s != nil {
		t.Fatalf("got %#v", s)
	}
}

func TestPriorityRank(t *testing.T) {
	t.Parallel()
	if got := priorityRank("P0"); got != 0 {
		t.Fatalf("P0 rank: got %d", got)
	}
	if got := priorityRank("P3"); got != 3 {
		t.Fatalf("P3 rank: got %d", got)
	}
	if got := priorityRank("UNKNOWN"); got != 9 {
		t.Fatalf("default rank: got %d", got)
	}
}

func TestResolveSession_EnvOverridesStateFile(t *testing.T) {
	projectRoot := t.TempDir()
	stateDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.EnsureDir(stateDir); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(stateDir, "session"), []byte("ZQK-file\n")); err != nil {
		t.Fatalf("write session file: %v", err)
	}
	t.Setenv(zqkenv.SessionID(), "ZQK-env")

	id, src := resolveSession(projectRoot)
	if id != "ZQK-env" || src != "env" {
		t.Fatalf("resolveSession env precedence: id=%q src=%q", id, src)
	}
}

func TestResolveSession_StateFileFallback(t *testing.T) {
	projectRoot := t.TempDir()
	stateDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.EnsureDir(stateDir); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(stateDir, "session"), []byte("ZQK-file\n")); err != nil {
		t.Fatalf("write session file: %v", err)
	}
	t.Setenv(zqkenv.SessionID(), "")

	id, src := resolveSession(projectRoot)
	if id != "ZQK-file" || src != "state_file" {
		t.Fatalf("resolveSession file fallback: id=%q src=%q", id, src)
	}
}

func TestSelectPolicyInterrupt_ReturnsLatestUnackedCritical(t *testing.T) {
	projectRoot := t.TempDir()
	if err := policyinterrupt.AppendInterrupt(projectRoot, policyinterrupt.InterruptRecord{
		Severity:    policyinterrupt.SeverityCritical,
		AckRequired: true,
		DedupeKey:   "DEDUPE-OLD",
		Message:     "old",
	}); err != nil {
		t.Fatalf("append old interrupt: %v", err)
	}
	if err := policyinterrupt.AppendInterrupt(projectRoot, policyinterrupt.InterruptRecord{
		Severity:    policyinterrupt.SeverityCritical,
		AckRequired: true,
		DedupeKey:   "DEDUPE-NEW",
		Message:     "new",
	}); err != nil {
		t.Fatalf("append new interrupt: %v", err)
	}
	got, ok := selectPolicyInterrupt(projectRoot)
	if !ok {
		t.Fatal("expected policy decision")
	}
	got, ok = nildecode.DecodeNonNilPayload[*policyDecision](got)
	if !ok {
		t.Fatal("expected policy decision")
	}
	if got.DedupeKey != "DEDUPE-NEW" {
		t.Fatalf("dedupe key: got %q", got.DedupeKey)
	}
	if err := policyinterrupt.AppendAck(projectRoot, policyinterrupt.AckRecord{
		DedupeKey: "DEDUPE-NEW",
	}); err != nil {
		t.Fatalf("append ack: %v", err)
	}
	got2, ok2 := selectPolicyInterrupt(projectRoot)
	if !ok2 {
		t.Fatal("expected fallback to older unacked critical")
	}
	got2, ok2 = nildecode.DecodeNonNilPayload[*policyDecision](got2)
	if !ok2 {
		t.Fatal("expected fallback to older unacked critical")
	}
	if got2.DedupeKey != "DEDUPE-OLD" {
		t.Fatalf("fallback dedupe key: got %q", got2.DedupeKey)
	}
}
