package docman

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestDocmanVerify_CLI(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SkipSetupTestEnvironment: false, SkipFileStorage: false})
	tmpDir := proj.Root

	docPath := "docs/architecture/verify_sample.md"
	fullDoc := filepath.Join(tmpDir, docPath)
	if err := fileutil.MkdirAll(filepath.Dir(fullDoc), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	initialContent := "# Verify Sample\nContent line for verification test."
	if err := fileutil.WriteFile(fullDoc, []byte(initialContent), paths.FilePerm644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	t.Setenv(zqkenv.ProjectRoot().Name(), tmpDir)
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)

	originalDir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	defer fileutil.Chdir(originalDir)
	if err := fileutil.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	// 1. Register the doc via CLI
	regCmd := NewRegisterCmd()
	regCmd.SetArgs([]string{"--shipped-only"})
	if err := regCmd.Execute(); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// 2. Run verify --shipped-only -> should succeed immediately since register seals hashes
	verifyCmd := NewVerifyCmd()
	var outBuf bytes.Buffer
	verifyCmd.SetOut(&outBuf)
	verifyCmd.SetArgs([]string{"--shipped-only"})
	if err := verifyCmd.Execute(); err != nil {
		t.Fatalf("verify failed on pristine docs: %v, output: %s", err, outBuf.String())
	}

	// 3. Tamper with file to introduce cryptographic drift
	tamperedContent := "# Verify Sample\nTampered content!"
	if err := fileutil.WriteFile(fullDoc, []byte(tamperedContent), paths.FilePerm644); err != nil {
		t.Fatalf("write tampered file failed: %v", err)
	}

	// 4. Run verify in strict mode -> must fail closed
	strictCmd := NewVerifyCmd()
	var strictBuf bytes.Buffer
	strictCmd.SetOut(&strictBuf)
	strictCmd.SetArgs([]string{"--shipped-only", "--strict=true"})
	if err := strictCmd.Execute(); err == nil {
		t.Fatalf("expected verify to fail on drifted content, but succeeded! output: %s", strictBuf.String())
	}

	// 5. Run verify with --auto-seal -> should heal the drift
	autoSealCmd := NewVerifyCmd()
	var autoSealBuf bytes.Buffer
	autoSealCmd.SetOut(&autoSealBuf)
	autoSealCmd.SetArgs([]string{"--shipped-only", "--auto-seal"})
	if err := autoSealCmd.Execute(); err != nil {
		t.Fatalf("verify --auto-seal failed: %v, output: %s", err, autoSealBuf.String())
	}

	// 6. Verify once more in strict mode -> should pass
	finalCmd := NewVerifyCmd()
	var finalBuf bytes.Buffer
	finalCmd.SetOut(&finalBuf)
	finalCmd.SetArgs([]string{"--shipped-only", "--strict=true"})
	if err := finalCmd.Execute(); err != nil {
		t.Fatalf("final verify failed after auto-seal: %v, output: %s", err, finalBuf.String())
	}
}
