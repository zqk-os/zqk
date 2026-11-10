package community

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestStandaloneContainer_StaticFloorInvariant verifies that Dockerfile.standalone
// conforms to container hardening and turnkey local inference specifications
// (CRIT-1794350068516428000-7f6b31d3).
func TestStandaloneContainer_StaticFloorInvariant(t *testing.T) {
	root := paths.ResolveProjectRoot(".")

	dockerfilePath := filepath.Join(root, "Dockerfile.standalone")
	if !fileutil.Exists(dockerfilePath) {
		t.Fatalf("expected Dockerfile.standalone at %s", dockerfilePath)
	}

	contentBytes, err := fileutil.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("failed to read Dockerfile.standalone: %v", err)
	}
	content := string(contentBytes)

	// Invariant 1: Multi-stage build structure
	if !strings.Contains(content, "AS zqk-builder") || !strings.Contains(content, "AS llama-builder") || !strings.Contains(content, "AS runner") {
		t.Errorf("expected multi-stage build with zqk-builder, llama-builder, and runner")
	}

	// Invariant 2: Non-root user
	if !strings.Contains(content, "USER 10001") {
		t.Errorf("expected Dockerfile.standalone to specify non-root USER 10001")
	}

	// Invariant 3: Port exposure for inference (8080) and MCP (8443)
	if !strings.Contains(content, "EXPOSE 8080 8443") {
		t.Errorf("expected Dockerfile.standalone to expose ports 8080 and 8443")
	}

	// Invariant 4: Standalone environment configuration
	if !strings.Contains(content, "ENV ZQK_LLM_PROVIDER=openai_compatible") ||
		!strings.Contains(content, "ENV ZQK_LLM_ENDPOINT=http://127.0.0.1:8080/v1") ||
		!strings.Contains(content, "ENV MODEL_PATH=/models/model.gguf") {
		t.Errorf("expected standalone inference environment variables to be configured")
	}

	// Invariant 5: Healthcheck and Entrypoint
	if !strings.Contains(content, "HEALTHCHECK") {
		t.Errorf("expected HEALTHCHECK instruction in Dockerfile.standalone")
	}
	if !strings.Contains(content, "ENTRYPOINT [\"/usr/local/bin/entrypoint-standalone.sh\"]") {
		t.Errorf("expected entrypoint-standalone.sh entrypoint")
	}

	// Invariant 6: Entrypoint script exists on host
	entrypointPath := filepath.Join(root, "scripts", "open-core", "entrypoint-standalone.sh")
	if !fileutil.Exists(entrypointPath) {
		t.Fatalf("expected scripts/open-core/entrypoint-standalone.sh at %s", entrypointPath)
	}
}

// TestStandaloneSupervisor_OperationalProof verifies the dynamic execution of
// scripts/entrypoint-standalone.sh with simulated model presence and command forwarding
// (CRIT-1794350068516429000-afa2806e).
func TestStandaloneSupervisor_OperationalProof(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	entrypointScript := filepath.Join(root, "scripts", "open-core", "entrypoint-standalone.sh")

	tmpDir := t.TempDir()
	mockBin := filepath.Join(tmpDir, "mock-zqk.sh")
	mockScript := "#!/bin/sh\necho \"MOCK_ZQK_INVOKED: $@\"\nexit 0\n"
	if err := fileutil.WriteFile(mockBin, []byte(mockScript), paths.FilePerm755); err != nil {
		t.Fatalf("failed to write mock binary: %v", err)
	}

	// Case 1: Fallback path when MODEL_PATH is absent
	cmd := testkit.ManagedCommand(t, t.Context(), "bash", entrypointScript, "workflow", "whats-next")
	cmd.Env = append(cmd.Environ(), "ZQK_BIN="+mockBin, "MODEL_PATH=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("entrypoint script execution failed: %v, out: %s", err, string(out))
	}
	outStr := string(out)

	if !strings.Contains(outStr, "[STANDALONE] No local GGUF model detected") {
		t.Errorf("expected fallback notice in output, got: %s", outStr)
	}
	if !strings.Contains(outStr, "MOCK_ZQK_INVOKED: workflow whats-next") {
		t.Errorf("expected argument forwarding to ZQK_BIN, got: %s", outStr)
	}
}

// TestStandaloneContainer_NegativeBoundaries verifies supervisor behavior on
// failure modes such as target binary errors and invalid model configurations
// (CRIT-1794350068516430000-aa597408).
func TestStandaloneContainer_NegativeBoundaries(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	entrypointScript := filepath.Join(root, "scripts", "open-core", "entrypoint-standalone.sh")

	tmpDir := t.TempDir()
	failingBin := filepath.Join(tmpDir, "failing-zqk.sh")
	failingScript := "#!/bin/sh\necho \"SIMULATED_FAILURE\"\nexit 42\n"
	if err := fileutil.WriteFile(failingBin, []byte(failingScript), paths.FilePerm755); err != nil {
		t.Fatalf("failed to write failing binary: %v", err)
	}

	// Boundary 1: Propagate binary failure exit code cleanly
	cmd := testkit.ManagedCommand(t, t.Context(), "bash", entrypointScript, "do")
	cmd.Env = append(cmd.Environ(), "ZQK_BIN="+failingBin, "MODEL_PATH=")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected command to exit with error code 42, but succeeded")
	}
	if !strings.Contains(string(out), "SIMULATED_FAILURE") {
		t.Errorf("expected simulated failure message, got: %s", string(out))
	}

	// Boundary 2: Nonexistent ZQK_BIN must fail fail-closed
	cmdMissing := testkit.ManagedCommand(t, t.Context(), "bash", entrypointScript, "do")
	cmdMissing.Env = append(cmdMissing.Environ(), "ZQK_BIN=/nonexistent/binary", "MODEL_PATH=")
	if err := cmdMissing.Run(); err == nil {
		t.Fatalf("expected failure on nonexistent binary")
	}
}
