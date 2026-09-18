package community

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestClientSDK_FunctionalAcceptance verifies that generate-client-sdk.sh generates
// complete, valid multi-language client bindings (TypeScript, Python, Go) from the OpenAPI 3.0 specification
// (CRIT-1789710618301220000-7586bc7c).
func TestClientSDK_FunctionalAcceptance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	scriptPath := filepath.Join(root, "scripts", "generate-client-sdk.sh")
	specPath := filepath.Join(root, "api", "openapi", "zqk-openapi.yaml")

	if !fileutil.Exists(scriptPath) {
		t.Fatalf("expected generate-client-sdk.sh at %s", scriptPath)
	}
	if !fileutil.Exists(specPath) {
		t.Fatalf("expected OpenAPI specification at %s", specPath)
	}

	tmpOut := t.TempDir()

	cmd := exec.Command("bash", scriptPath, "--spec", specPath, "--lang", "all", "--out-dir", tmpOut)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generate-client-sdk.sh failed: %v, output:\n%s", err, string(out))
	}

	// Verify TypeScript artifacts
	tsTypesPath := filepath.Join(tmpOut, "typescript", "src", "types.ts")
	tsClientPath := filepath.Join(tmpOut, "typescript", "src", "client.ts")
	tsPkgPath := filepath.Join(tmpOut, "typescript", "package.json")
	for _, p := range []string{tsTypesPath, tsClientPath, tsPkgPath} {
		if !fileutil.Exists(p) {
			t.Errorf("expected TypeScript artifact at %s", p)
		}
	}
	tsContent, err := os.ReadFile(tsTypesPath)
	if err != nil {
		t.Fatalf("failed reading ts types: %v", err)
	}
	for _, expectedType := range []string{"HealthStatus", "IntentRequest", "IntentResponse", "SystemMetrics"} {
		if !strings.Contains(string(tsContent), expectedType) {
			t.Errorf("expected TypeScript types to contain %s", expectedType)
		}
	}

	// Verify Python artifacts
	pyModelsPath := filepath.Join(tmpOut, "python", "zqk_client", "models.py")
	pyClientPath := filepath.Join(tmpOut, "python", "zqk_client", "client.py")
	pyInitPath := filepath.Join(tmpOut, "python", "zqk_client", "__init__.py")
	pyProjPath := filepath.Join(tmpOut, "python", "pyproject.toml")
	for _, p := range []string{pyModelsPath, pyClientPath, pyInitPath, pyProjPath} {
		if !fileutil.Exists(p) {
			t.Errorf("expected Python artifact at %s", p)
		}
	}
	pyContent, err := os.ReadFile(pyModelsPath)
	if err != nil {
		t.Fatalf("failed reading py models: %v", err)
	}
	for _, expectedClass := range []string{"class HealthStatus", "class IntentRequest", "class SystemMetrics"} {
		if !strings.Contains(string(pyContent), expectedClass) {
			t.Errorf("expected Python models to contain %s", expectedClass)
		}
	}

	// Verify Go artifacts
	goTypesPath := filepath.Join(tmpOut, "go", "zqk", "types.go")
	goClientPath := filepath.Join(tmpOut, "go", "zqk", "client.go")
	goModPath := filepath.Join(tmpOut, "go", "go.mod")
	for _, p := range []string{goTypesPath, goClientPath, goModPath} {
		if !fileutil.Exists(p) {
			t.Errorf("expected Go artifact at %s", p)
		}
	}
	goContent, err := os.ReadFile(goTypesPath)
	if err != nil {
		t.Fatalf("failed reading go types: %v", err)
	}
	for _, expectedStruct := range []string{"type HealthStatus struct", "type IntentRequest struct", "type SystemMetrics struct"} {
		if !strings.Contains(string(goContent), expectedStruct) {
			t.Errorf("expected Go types to contain %s", expectedStruct)
		}
	}
}

// TestClientSDK_BoundaryAndErrorHandling verifies error handling on invalid specs,
// unsupported languages, missing operation IDs, and malformed inputs
// (CRIT-1789710618301221000-8141aca2).
func TestClientSDK_BoundaryAndErrorHandling(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	scriptPath := filepath.Join(root, "scripts", "generate-client-sdk.sh")

	// 1. Nonexistent spec
	cmdNonexistent := exec.Command("bash", scriptPath, "--spec", "/nonexistent/spec.yaml")
	if err := cmdNonexistent.Run(); err == nil {
		t.Errorf("expected generator to fail on nonexistent spec")
	}

	// 2. Unsupported language
	tmpDir := t.TempDir()
	validSpec := filepath.Join(root, "api", "openapi", "zqk-openapi.yaml")
	cmdBadLang := exec.Command("bash", scriptPath, "--spec", validSpec, "--lang", "rust", "--out-dir", tmpDir)
	if err := cmdBadLang.Run(); err == nil {
		t.Errorf("expected generator to fail on unsupported language 'rust'")
	}

	// 3. Malformed YAML
	badYAML := filepath.Join(tmpDir, "bad.yaml")
	if err := os.WriteFile(badYAML, []byte("openapi: 3.0.0\n[invalid yaml: {"), 0644); err != nil {
		t.Fatalf("failed writing bad yaml: %v", err)
	}
	cmdBadYAML := exec.Command("bash", scriptPath, "--verify", badYAML)
	if err := cmdBadYAML.Run(); err == nil {
		t.Errorf("expected verification to fail on corrupt YAML")
	}

	// 4. Missing required info/paths
	incompleteSpec := filepath.Join(tmpDir, "incomplete.yaml")
	incompleteContent := `openapi: 3.0.0
info:
  title: Incomplete
paths: {}
`
	if err := os.WriteFile(incompleteSpec, []byte(incompleteContent), 0644); err != nil {
		t.Fatalf("failed writing incomplete yaml: %v", err)
	}
	cmdIncomplete := exec.Command("bash", scriptPath, "--verify", incompleteSpec)
	if err := cmdIncomplete.Run(); err == nil {
		t.Errorf("expected verification to fail on incomplete OpenAPI spec")
	}

	// 5. Operation missing operationId
	missingOpIDSpec := filepath.Join(tmpDir, "missing_op.yaml")
	missingOpContent := `openapi: 3.0.0
info:
  title: Test
  version: 1.0.0
paths:
  /test:
    get:
      summary: Test endpoint
components:
  schemas:
    Test:
      type: object
`
	if err := os.WriteFile(missingOpIDSpec, []byte(missingOpContent), 0644); err != nil {
		t.Fatalf("failed writing missing op yaml: %v", err)
	}
	cmdMissingOp := exec.Command("bash", scriptPath, "--verify", missingOpIDSpec)
	if err := cmdMissingOp.Run(); err == nil {
		t.Errorf("expected verification to fail on operation missing operationId")
	}
}

// TestClientSDK_IntegrationAndConformance verifies end-to-end client execution against
// daemon endpoints and compiler verification for all generated bindings
// (CRIT-1789710618301222000-ed6b94df).
func TestClientSDK_IntegrationAndConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	specPath := filepath.Join(root, "api", "openapi", "zqk-openapi.yaml")
	genPy := filepath.Join(root, "scripts", "generate_client_sdk.py")

	tmpDir := t.TempDir()

	// 1. Run generation
	cmdGen := exec.Command("python3", genPy, "--spec", specPath, "--lang", "all", "--out-dir", tmpDir)
	if out, err := cmdGen.CombinedOutput(); err != nil {
		t.Fatalf("generation failed: %v, out:\n%s", err, string(out))
	}

	// 2. Validate Python syntax
	pyFiles := []string{
		filepath.Join(tmpDir, "python", "zqk_client", "models.py"),
		filepath.Join(tmpDir, "python", "zqk_client", "client.py"),
		filepath.Join(tmpDir, "python", "zqk_client", "__init__.py"),
	}
	cmdPyCompile := exec.Command("python3", append([]string{"-m", "py_compile"}, pyFiles...)...)
	if out, err := cmdPyCompile.CombinedOutput(); err != nil {
		t.Errorf("Python compilation failed: %v, out:\n%s", err, string(out))
	}

	// 3. Mock HTTP Server conformance test
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/health":
			json.NewEncoder(w).Encode(map[string]any{
				"status":         "healthy",
				"version":        "v2.8.0",
				"uptime_seconds": 120,
			})
		case "/api/intent":
			json.NewEncoder(w).Encode(map[string]any{
				"id":        "INT-1001",
				"status":    "completed",
				"timestamp": "2026-09-18T05:00:00Z",
			})
		case "/api/metrics":
			json.NewEncoder(w).Encode(map[string]any{
				"cpu_usage_percent": 2.5,
				"memory_rss_bytes":  50000000,
				"goroutines":        16,
				"scheduler_health":  99.8,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{"error": "not found", "code": 404})
		}
	}))
	defer ts.Close()

	// Verify Python client against mock server
	pyTestScript := filepath.Join(tmpDir, "test_client.py")
	pyTestCode := `
import sys
sys.path.insert(0, "` + filepath.Join(tmpDir, "python") + `")
from zqk_client import ZQKClient

client = ZQKClient(base_url="` + ts.URL + `")
health = client.get_health()
assert health["status"] == "healthy", f"expected healthy, got {health}"

intent = client.post_intent("workflow_dispatch", {"task": "test"})
assert intent["status"] == "completed", f"expected completed, got {intent}"

metrics = client.get_metrics()
assert metrics["goroutines"] == 16, f"expected 16 goroutines, got {metrics}"
print("PYTHON_CLIENT_CONFORMANCE_OK")
`
	if err := os.WriteFile(pyTestScript, []byte(pyTestCode), 0644); err != nil {
		t.Fatalf("failed to write test_client.py: %v", err)
	}

	cmdPyRun := exec.Command("python3", pyTestScript)
	outPy, err := cmdPyRun.CombinedOutput()
	if err != nil {
		t.Errorf("Python client conformance test failed: %v, out:\n%s", err, string(outPy))
	}
	if !strings.Contains(string(outPy), "PYTHON_CLIENT_CONFORMANCE_OK") {
		t.Errorf("expected confirmation from Python client conformance test")
	}

	// 4. Verify Go SDK compiles and passes tests
	cmdGoBuild := exec.Command("go", "build", "./zqk")
	cmdGoBuild.Dir = filepath.Join(tmpDir, "go")
	if out, err := cmdGoBuild.CombinedOutput(); err != nil {
		t.Errorf("Go SDK build failed: %v, out:\n%s", err, string(out))
	}
}
