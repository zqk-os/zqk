package test_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/cmd/zqk/test"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func init() {
	_ = os.Setenv(zqkenv.ZQKAllowForegroundGoTest().Key, "1")
}

func TestDiscoverCmd_Basic(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "cmd.test.discover"})

	testFile := `package dummy_test
import "testing"
func TestDummyItem(t *testing.T) {}
`
	if err := os.WriteFile(filepath.Join(proj.Root, "dummy_test.go"), []byte(testFile), paths.FilePerm600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cmd := test.NewDiscoverCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{proj.Root})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("discover command failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "TestDummyItem") {
		t.Errorf("expected output to contain TestDummyItem, got:\n%s", out)
	}
}

func TestDiscoverCmd_JSONFormat(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "cmd.test.discover.json"})

	testFile := `package dummy_test
import "testing"
func TestDummyJson(t *testing.T) {}
`
	if err := os.WriteFile(filepath.Join(proj.Root, "dummy_test.go"), []byte(testFile), paths.FilePerm600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cmd := test.NewDiscoverCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{proj.Root, "--format", "json"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("discover command failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"function": "TestDummyJson"`) {
		t.Errorf("expected JSON output with function TestDummyJson, got:\n%s", out)
	}
}
