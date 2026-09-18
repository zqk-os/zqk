package releasegate

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/opencore"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestPublicPayloadCheck_FunctionalAcceptance verifies that clean open-core directory trees
// pass the opencore PayloadCheck and shell release payload gates (CRIT-1789626190967619000-f9fd6104).
func TestPublicPayloadCheck_FunctionalAcceptance(t *testing.T) {
	tmpDir := t.TempDir()

	// Populate clean tree with allowed files
	if err := fileutil.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Community Open-Core\nSafe release payload."), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(tmpDir, "LICENSE"), []byte("Apache License 2.0"), 0644); err != nil {
		t.Fatalf("failed to write LICENSE: %v", err)
	}
	codeDir := filepath.Join(tmpDir, "pkg", "tool")
	if err := fileutil.EnsureDir(codeDir); err != nil {
		t.Fatalf("failed to create code dir: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(codeDir, "tool.go"), []byte("package tool\nfunc Run() string { return \"ok\" }\n"), 0644); err != nil {
		t.Fatalf("failed to write go code: %v", err)
	}

	// 1. Go opencore payload scan
	report, err := opencore.PayloadCheck(tmpDir, opencore.PayloadCheckOptions{
		CheckBinaries: true,
	})
	if err != nil {
		t.Fatalf("PayloadCheck returned error on clean tree: %v", err)
	}
	if report.TotalViolations > 0 {
		t.Errorf("expected 0 violations for clean tree, got %d: %+v", report.TotalViolations, report.Violations)
	}

	// 2. Shell payload check script
	root := findModuleRoot(t)
	script := filepath.Join(root, "scripts", "check-public-release-payload.sh")
	if !fileutil.Exists(script) {
		t.Fatalf("check-public-release-payload.sh not found at %s", script)
	}

	// Set up remote_hold.json
	stateDir := filepath.Join(tmpDir, ".zqk", "state")
	if err := fileutil.EnsureDir(stateDir); err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}
	holdJSON := `{"hold": true, "` + objects.FieldKeyReason + `": "Default deny"}`
	if err := fileutil.WriteFile(filepath.Join(stateDir, "remote_hold.json"), []byte(holdJSON), 0644); err != nil {
		t.Fatalf("failed to write remote_hold.json: %v", err)
	}

	cmd := execwrap.Command("sh", script, tmpDir)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("check-public-release-payload.sh failed on clean tree: %v\nOutput:\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "RESULT=PASS") {
		t.Errorf("expected RESULT=PASS in output, got:\n%s", string(out))
	}
}

// TestPublicPayloadCheck_BoundaryAndErrorHandling verifies fail-closed rejection of forbidden
// instance data, long studio IDs, and sensitive secrets (CRIT-1789626190967620000-74dcba74).
func TestPublicPayloadCheck_BoundaryAndErrorHandling(t *testing.T) {
	root := findModuleRoot(t)
	script := filepath.Join(root, "scripts", "check-public-release-payload.sh")
	if !fileutil.Exists(script) {
		t.Fatalf("check-public-release-payload.sh not found at %s", script)
	}

	// Case 1: Rejects forbidden process instance data
	t.Run("RejectsProcessInstanceData", func(t *testing.T) {
		tmpDir := t.TempDir()
		bliDir := filepath.Join(tmpDir, ".zqk", "process", "backlog_items")
		if err := fileutil.EnsureDir(bliDir); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		if err := fileutil.WriteFile(filepath.Join(bliDir, "item.yaml"), []byte("id: BLI-1785642072074959000-a1dfbde9\n"), 0644); err != nil {
			t.Fatalf("write failed: %v", err)
		}

		cmd := execwrap.Command("sh", script, tmpDir)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("expected non-zero exit for process instance data, got success:\n%s", string(out))
		}
		if !strings.Contains(string(out), "BLOCK:") {
			t.Errorf("expected BLOCK: in output, got:\n%s", string(out))
		}
	})

	// Case 2: Rejects unscrubbed long studio kernel IDs in source files
	t.Run("RejectsLongStudioKernelIDsInSource", func(t *testing.T) {
		tmpDir := t.TempDir()
		pkgDir := filepath.Join(tmpDir, "pkg", "core")
		if err := fileutil.EnsureDir(pkgDir); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		if err := fileutil.WriteFile(filepath.Join(pkgDir, "core.go"), []byte("// Ref: BLI-1785642072074959000-a1dfbde9\npackage core\n"), 0644); err != nil {
			t.Fatalf("write failed: %v", err)
		}

		cmd := execwrap.Command("sh", script, tmpDir)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("expected non-zero exit for studio kernel ID in source, got success:\n%s", string(out))
		}
		if !strings.Contains(string(out), "BLOCK:") {
			t.Errorf("expected BLOCK: in output, got:\n%s", string(out))
		}
	})

	// Case 3: Opencore scanner catches secrets and binaries
	t.Run("OpencoreCatchesSecretsAndBinaries", func(t *testing.T) {
		tmpDir := t.TempDir()
		if err := fileutil.WriteFile(filepath.Join(tmpDir, "secret.txt"), []byte("api_key = \"sk-1234567890abcdef\"\n"), 0644); err != nil {
			t.Fatalf("write failed: %v", err)
		}
		if err := fileutil.WriteFile(filepath.Join(tmpDir, "blob.bin"), []byte{0x7f, 0x45, 0x4c, 0x46, 0x01}, 0644); err != nil {
			t.Fatalf("write failed: %v", err)
		}

		report, err := opencore.PayloadCheck(tmpDir, opencore.PayloadCheckOptions{
			CheckBinaries: true,
		})
		if err != nil {
			t.Fatalf("PayloadCheck returned error: %v", err)
		}
		if report.TotalViolations < 2 {
			t.Errorf("expected >= 2 violations (secret + binary), got %d: %+v", report.TotalViolations, report.Violations)
		}
	})
}

// TestPublicPayloadCheck_IntegrationAndConformance runs the full public push fail-closed gate audit
// to confirm end-to-end conformance with security baseline controls (CRIT-1789626190967621000-d3650801).
func TestPublicPayloadCheck_IntegrationAndConformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration and conformance check in -short mode")
	}

	root := findModuleRoot(t)
	script := filepath.Join(root, "scripts", "test-public-push-gate-failclosed.sh")
	if !fileutil.Exists(script) {
		t.Fatalf("test-public-push-gate-failclosed.sh not found at %s", script)
	}

	cmd := execwrap.Command("bash", script)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("test-public-push-gate-failclosed.sh failed: %v\nOutput:\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "All Public Push Fail-Closed Checks PASSED") {
		t.Errorf("expected success banner, got:\n%s", string(out))
	}
}
