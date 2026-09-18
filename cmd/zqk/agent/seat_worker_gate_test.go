package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/swarm"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func writeRecord(path string) swarm.ToolCallRecord {
	return swarm.ToolCallRecord{
		Name:      swarm.MutationEvidenceTools()[0],
		Arguments: mustJSON(map[string]string{objects.FieldKeyPath: path}),
	}
}

func objectGetArgs(id string) string {
	return mustJSON(map[string]string{objects.FieldKeyID: id})
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestGoModulePath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if got := goModulePath(root); got != "the path declared in go.mod" {
		t.Fatalf("missing go.mod = %q", got)
	}
	mustWriteFile(t, filepath.Join(root, "go.mod"), "module github.com/zqk-os/zqk\n\ngo 1.26\n")
	if got := goModulePath(root); got != "github.com/zqk-os/zqk" {
		t.Fatalf("go.mod module = %q", got)
	}
}

func TestGoFilesWritten_acceptsStableChannelToolName(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	got := goFilesWritten([]swarm.ToolCallRecord{
		{Name: "zqk-stable_write_file", Arguments: mustJSON(map[string]string{objects.FieldKeyPath: "pkg/foo/foo.go"})},
		{Name: "zqk_write_code", Arguments: mustJSON(map[string]string{objects.FieldKeyPath: "pkg/bar/bar.go"})},
	}, root)
	if len(got) != 2 {
		t.Fatalf("go files = %v, want two writes under either prefix", got)
	}
}

func TestGoPackagesWritten(t *testing.T) {
	t.Parallel()
	root := "/repo"

	got := goPackagesWritten([]swarm.ToolCallRecord{
		writeRecord("pkg/scheduler/auth_hook.go"),
		writeRecord("pkg/scheduler/auth_hook_test.go"),
		writeRecord("cmd/zqk/agent/seat_worker.go"),
		writeRecord("docs/notes.md"),
		{Name: "zqk_object_get", Arguments: objectGetArgs("ATK-1")},
	}, root)

	want := []string{"./cmd/zqk/agent", "./pkg/scheduler"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("packages = %v, want %v", got, want)
	}
}

func TestGoPackagesWritten_ignoresPathsOutsideRoot(t *testing.T) {
	t.Parallel()
	if got := goPackagesWritten([]swarm.ToolCallRecord{writeRecord("/etc/evil.go")}, "/repo"); len(got) != 0 {
		t.Fatalf("packages = %v, want none", got)
	}
}

// The gate must reject work that does not compile, which is exactly what a
// successful write tool call cannot tell us.
func TestVerifyGoWorkAsCompletion_rejectsCodeThatDoesNotCompile(t *testing.T) {
	root := t.TempDir()
	mustWriteGoModule(t, root)
	mustWriteFile(t, filepath.Join(root, "widget", "widget.go"), `package widget

func Broken() string {
	return undefinedHelper()
}
`)

	feedback, err := verifyGoWorkAsCompletion(context.Background(), root, []swarm.ToolCallRecord{
		writeRecord("widget/widget.go"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(feedback, "do not compile") {
		t.Fatalf("feedback = %q, want a compile rejection", feedback)
	}
	if !strings.Contains(feedback, "undefinedHelper") {
		t.Fatalf("feedback should quote the compiler error, got %q", feedback)
	}
	if !strings.Contains(feedback, "gatecheck") {
		t.Fatalf("feedback should name the go.mod module, got %q", feedback)
	}
}

func TestVerifyGoWorkAsCompletion_rejectsFailingTests(t *testing.T) {
	root := t.TempDir()
	mustWriteGoModule(t, root)
	mustWriteFile(t, filepath.Join(root, "widget", "widget.go"), `package widget

func Answer() int { return 41 }
`)
	mustWriteFile(t, filepath.Join(root, "widget", "widget_test.go"), `package widget

import "testing"

func TestAnswer(t *testing.T) {
	if Answer() != 42 {
		t.Fatal("wrong answer")
	}
}
`)

	feedback, err := verifyGoWorkAsCompletion(context.Background(), root, []swarm.ToolCallRecord{
		writeRecord("widget/widget.go"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(feedback, "tests fail") {
		t.Fatalf("feedback = %q, want a test rejection", feedback)
	}
}

func TestVerifyGoWorkAsCompletion_acceptsWorkingCode(t *testing.T) {
	root := t.TempDir()
	mustWriteGoModule(t, root)
	mustWriteFile(t, filepath.Join(root, "widget", "widget.go"), `package widget

func Answer() int { return 42 }
`)
	mustWriteFile(t, filepath.Join(root, "widget", "widget_test.go"), `package widget

import "testing"

func TestAnswer(t *testing.T) {
	if Answer() != 42 {
		t.Fatal("wrong answer")
	}
}
`)

	feedback, err := verifyGoWorkAsCompletion(context.Background(), root, []swarm.ToolCallRecord{
		writeRecord("widget/widget.go"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if feedback != "" {
		t.Fatalf("feedback = %q, want acceptance", feedback)
	}
}

func TestVerifyGoWorkAsCompletion_rejectsNoMutationWrites(t *testing.T) {
	t.Parallel()
	feedback, err := verifyGoWorkAsCompletion(context.Background(), t.TempDir(), []swarm.ToolCallRecord{
		{Name: "zqk_object_get", Arguments: objectGetArgs("ATK-1")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(feedback, "mutation tool") {
		t.Fatalf("feedback = %q, want a mutation-evidence rejection", feedback)
	}
}

func TestVerifyGoWorkAsCompletion_acceptsNonGoMutationWrite(t *testing.T) {
	t.Parallel()
	feedback, err := verifyGoWorkAsCompletion(context.Background(), t.TempDir(), []swarm.ToolCallRecord{
		writeRecord("scripts/forbid-git-clean-process.sh"),
	})
	if err != nil || feedback != "" {
		t.Fatalf("feedback = %q err = %v, want acceptance for a script write", feedback, err)
	}
}

func TestRevertSeatWrites_removesCreatedAndRestoresTracked(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "gate@test")
	runGit(t, root, "config", "user.name", "gate")

	tracked := filepath.Join(root, "pkg", "kept.go")
	mustWriteFile(t, tracked, "package kept\n\nconst Original = 1\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "baseline")

	// A seat clobbers a tracked file and creates a new one.
	mustWriteFile(t, tracked, "package kept\n\nconst Clobbered = 2\n")
	created := filepath.Join(root, "pkg", "created.go")
	mustWriteFile(t, created, "package kept\n")

	if err := revertSeatWrites(context.Background(), root, []string{"pkg/kept.go", "pkg/created.go"}); err != nil {
		t.Fatal(err)
	}

	restored, err := fileutil.ReadFile(tracked)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(restored), "Original") {
		t.Fatalf("tracked file not restored: %q", restored)
	}
	if _, err := fileutil.Stat(created); !fileutil.IsNotExist(err) {
		t.Fatalf("created file should be removed, stat err = %v", err)
	}
}

// Kernel CAS instance data is never rolled back through git.
func TestRevertSeatWrites_skipsProcessData(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	casFile := filepath.Join(root, paths.ProcessDir, "policies", "abc.go")
	mustWriteFile(t, casFile, "package p\n")

	if err := revertSeatWrites(context.Background(), root, []string{paths.ProcessDir + "/policies/abc.go"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fileutil.Stat(casFile); err != nil {
		t.Fatalf("process data must be left alone, stat err = %v", err)
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := execwrap.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func mustWriteGoModule(t *testing.T, root string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(root, "go.mod"), "module gatecheck\n\ngo 1.21\n")
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := fileutil.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
