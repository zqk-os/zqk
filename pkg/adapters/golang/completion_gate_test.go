package golang

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

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

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestModulePath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if got := modulePath(root); got != missingModuleLabel {
		t.Fatalf("missing go.mod = %q", got)
	}
	mustWriteFile(t, filepath.Join(root, "go.mod"), "module github.com/zqk-os/zqk\n\ngo 1.26\n")
	if got := modulePath(root); got != "github.com/zqk-os/zqk" {
		t.Fatalf("go.mod module = %q", got)
	}
}

func TestWrittenFiles_acceptsStableChannelToolName(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	got := CompletionGate{}.WrittenFiles([]swarm.ToolCallRecord{
		{Name: "zqk-stable_write_file", Arguments: mustJSON(map[string]string{objects.FieldKeyPath: "pkg/foo/foo.go"})},
		{Name: "zqk_write_code", Arguments: mustJSON(map[string]string{objects.FieldKeyPath: "pkg/bar/bar.go"})},
		{Name: "zqk_write_file", Arguments: mustJSON(map[string]string{objects.FieldKeyPath: "docs/notes.md"})},
	}, root)
	if len(got) != 2 {
		t.Fatalf("go files = %v, want two .go writes", got)
	}
}

func TestPackages(t *testing.T) {
	t.Parallel()
	root := "/repo"
	got := CompletionGate{}.Packages([]swarm.ToolCallRecord{
		writeRecord("pkg/scheduler/auth_hook.go"),
		writeRecord("pkg/scheduler/auth_hook_test.go"),
		writeRecord("cmd/zqk/agent/seat_worker.go"),
		writeRecord("docs/notes.md"),
		{Name: "zqk_object_get", Arguments: mustJSON(map[string]string{objects.FieldKeyID: "ATK-1"})},
	}, root)
	want := []string{"./cmd/zqk/agent", "./pkg/scheduler"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("packages = %v, want %v", got, want)
	}
}

func TestPackages_ignoresPathsOutsideRoot(t *testing.T) {
	t.Parallel()
	got := CompletionGate{}.Packages([]swarm.ToolCallRecord{writeRecord("/etc/evil.go")}, "/repo")
	if len(got) != 0 {
		t.Fatalf("packages = %v, want none", got)
	}
}

func TestVerify_rejectsCodeThatDoesNotCompile(t *testing.T) {
	root := t.TempDir()
	mustWriteGoModule(t, root)
	mustWriteFile(t, filepath.Join(root, "widget", "widget.go"), `package widget

func Broken() string {
	return undefinedHelper()
}
`)
	feedback, err := CompletionGate{}.Verify(context.Background(), root, []swarm.ToolCallRecord{
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

func TestVerify_rejectsFailingTests(t *testing.T) {
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
	feedback, err := CompletionGate{}.Verify(context.Background(), root, []swarm.ToolCallRecord{
		writeRecord("widget/widget.go"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(feedback, "tests fail") {
		t.Fatalf("feedback = %q, want a test rejection", feedback)
	}
}

func TestVerify_acceptsWorkingCode(t *testing.T) {
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
	feedback, err := CompletionGate{}.Verify(context.Background(), root, []swarm.ToolCallRecord{
		writeRecord("widget/widget.go"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if feedback != "" {
		t.Fatalf("feedback = %q, want acceptance", feedback)
	}
}

func TestVerify_acceptsNonGoMutationWrite(t *testing.T) {
	t.Parallel()
	feedback, err := CompletionGate{}.Verify(context.Background(), t.TempDir(), []swarm.ToolCallRecord{
		writeRecord("scripts/forbid-git-clean-process.sh"),
	})
	if err != nil || feedback != "" {
		t.Fatalf("feedback = %q err = %v, want acceptance for a script write", feedback, err)
	}
}

func mustWriteGoModule(t *testing.T, root string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(root, "go.mod"), "module gatecheck\n\ngo 1.21\n")
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(path, []byte(content), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
}
