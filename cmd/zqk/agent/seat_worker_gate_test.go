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

func TestVerifyWorkAsCompletion_rejectsNoMutationWrites(t *testing.T) {
	t.Parallel()
	feedback, err := verifyWorkAsCompletion(context.Background(), t.TempDir(), []swarm.ToolCallRecord{
		{Name: "zqk_object_get", Arguments: objectGetArgs("ATK-1")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(feedback, "mutation tool") {
		t.Fatalf("feedback = %q, want a mutation-evidence rejection", feedback)
	}
}

func TestVerifyWorkAsCompletion_acceptsNonGoMutationWrite(t *testing.T) {
	t.Parallel()
	feedback, err := verifyWorkAsCompletion(context.Background(), t.TempDir(), []swarm.ToolCallRecord{
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

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(path, []byte(content), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
}
