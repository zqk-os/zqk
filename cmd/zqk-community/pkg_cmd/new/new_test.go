package newcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestNewObject_stdout_contains_kind_and_header(t *testing.T) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"object", "backlog_item", "-o", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "kind: backlog_item") {
		t.Fatalf("expected backlog_item draft in stdout, got: %q", out)
	}
	if !strings.Contains(out, "zqk object create") {
		t.Fatalf("expected header comment with create hint, got: %q", out)
	}
}

func TestNewObject_kinds_emit_kind_field(t *testing.T) {
	kinds := []string{"backlog_item", "goal", "requirement"}
	for _, kind := range kinds {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			var buf bytes.Buffer
			ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
			cmd := NewNewCmd()
			cmd.SetContext(ctx)
			cmd.SetArgs([]string{"object", kind, "-o", "-"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			want := "kind: " + kind
			if !strings.Contains(buf.String(), want) {
				t.Fatalf("stdout missing %q:\n%s", want, buf.String())
			}
		})
	}
}

func TestNewObject_unknown_kind_errors(t *testing.T) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"object", "not_a_real_kind_xyz_12345", "-o", "-"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
	if !strings.Contains(err.Error(), "not_a_real_kind_xyz_12345") {
		t.Fatalf("error should mention kind: %v", err)
	}
}

func TestNewInternal_stdout_object_spec(t *testing.T) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"internal", "object_spec", "-o", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "kind: object_spec") {
		t.Fatalf("expected object_spec draft, got: %q", out)
	}
	if !strings.Contains(out, "zqk internal create") {
		t.Fatalf("expected internal create hint in header: %q", out)
	}
}

func TestNewObject_defaultOutput_createsDraftUnderZqkDrafts(t *testing.T) {
	tmp := t.TempDir()
	if err := fileutil.EnsureDir(filepath.Join(tmp, paths.ProjectDataDir)); err != nil {
		t.Fatal(err)
	}
	t.Setenv(zqkenv.ProjectRoot(), tmp)

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"object", "backlog_item"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(tmp, paths.ProjectDataDir, "drafts", "backlog_item-*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one backlog_item draft file, got %v (stdout=%q)", matches, buf.String())
	}
	b, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "kind: backlog_item") {
		t.Fatalf("draft content: %s", b)
	}
	ptrPath := filepath.Join(tmp, paths.ProjectDataDir, "drafts", "last-draft.yaml")
	ptrBytes, err := os.ReadFile(ptrPath)
	if err != nil {
		t.Fatalf("last-draft pointer: %v", err)
	}
	if !strings.Contains(string(ptrBytes), "scope: object") || !strings.Contains(string(ptrBytes), "kind: backlog_item") {
		t.Fatalf("unexpected pointer: %s", ptrBytes)
	}
}

func TestNewBundle_stdout_contains_scenario_bundle(t *testing.T) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"bundle", "-o", "-", "--name", "t-test"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "kind: scenario_bundle") {
		t.Fatalf("expected bundle kind in stdout, got: %q", out)
	}
}

// TestNewBundle_stdout_matches_NEW_COMMAND_contract asserts minimal scenario_bundle scaffold fields
// (docs/architecture/NEW_COMMAND_OBJECT_ORIGINATION.md).
func TestNewBundle_stdout_matches_NEW_COMMAND_contract(t *testing.T) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"bundle", "-o", "-", "--name", "contract-test", "--description", "e2e desc"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, needle := range []string{
		"api_version: v1",
		"kind: scenario_bundle",
		"metadata:",
		"name: contract-test",
		"description: e2e desc",
		"objects:",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("stdout missing %q:\n%s", needle, out)
		}
	}
}

func TestNewBundle_defaultOutput_createsDraftAndLastDraftPointer(t *testing.T) {
	tmp := t.TempDir()
	if err := fileutil.EnsureDir(filepath.Join(tmp, paths.ProjectDataDir)); err != nil {
		t.Fatal(err)
	}
	t.Setenv(zqkenv.ProjectRoot(), tmp)

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"bundle", "--name", "my-bundle"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(tmp, paths.ProjectDataDir, "drafts", "my-bundle-*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one bundle draft, got %v (stdout=%q)", matches, buf.String())
	}
	b, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "api_version: v1") || !strings.Contains(string(b), "objects:") {
		t.Fatalf("draft content: %s", b)
	}
	ptrPath := filepath.Join(tmp, paths.ProjectDataDir, "drafts", "last-draft.yaml")
	ptrBytes, err := os.ReadFile(ptrPath)
	if err != nil {
		t.Fatalf("last-draft pointer: %v", err)
	}
	s := string(ptrBytes)
	if !strings.Contains(s, "scope: bundle") || !strings.Contains(s, "kind: scenario_bundle") {
		t.Fatalf("unexpected pointer: %s", s)
	}
}

func TestNewInternal_unknown_kind_errors(t *testing.T) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"internal", "not_a_real_internal_kind_abc999", "-o", "-"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown internal kind")
	}
	if !strings.Contains(err.Error(), "not_a_real_internal_kind_abc999") {
		t.Fatalf("error should mention kind: %v", err)
	}
}

func TestNewObjectSpecKind_stdout_contains_inheritance_and_storage_profile(t *testing.T) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"object-spec", "draft_kind_xyz", "--extends", "base_object", "-o", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "ontology: draft_kind_xyz") {
		t.Fatalf("expected ontology in draft, got: %q", out)
	}
	if !strings.Contains(out, "Inheritance model") {
		t.Fatalf("expected inheritance comment block, got: %q", out)
	}
	if !strings.Contains(out, "storage_profile: cas_entity") {
		t.Fatalf("expected inherited storage_profile, got: %q", out)
	}
}
