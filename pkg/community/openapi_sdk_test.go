package community_test

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func findRepoRootForSDK(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repository root containing go.mod")
		}
		dir = parent
	}
}

// TestOpenAPIClientSDK_FunctionalAcceptance satisfies CRIT-1789710618301220000-7586bc7c.
// Verifies automated multi-language client SDK generation producing valid TypeScript, Python, and Go libraries.
func TestOpenAPIClientSDK_FunctionalAcceptance(t *testing.T) {
	repoRoot := findRepoRootForSDK(t)
	specPath := filepath.Join(repoRoot, "docs", "api", "openapi.yaml")
	if _, err := os.Stat(specPath); err != nil {
		t.Fatalf("OpenAPI specification file does not exist at %s: %v", specPath, err)
	}

	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "sdk")

	cmd := exec.Command("bash", filepath.Join(repoRoot, "scripts", "generate-openapi-clients.sh"), outDir)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generate-openapi-clients.sh failed: %v\nOutput:\n%s", err, string(out))
	}

	// Verify TypeScript SDK output
	tsIndex := filepath.Join(outDir, "typescript", "src", "index.ts")
	tsPkg := filepath.Join(outDir, "typescript", "package.json")
	tsCfg := filepath.Join(outDir, "typescript", "tsconfig.json")
	for _, f := range []string{tsIndex, tsPkg, tsCfg} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("expected TypeScript file missing: %s", f)
		}
	}

	tsBytes, err := os.ReadFile(tsIndex)
	if err != nil {
		t.Fatalf("failed reading ts index: %v", err)
	}
	tsContent := string(tsBytes)
	for _, sym := range []string{"export class ZqkClient", "export class ZqkApiError", "getHealth", "getVersion", "listObjects"} {
		if !strings.Contains(tsContent, sym) {
			t.Errorf("TypeScript SDK missing expected symbol/method: %q", sym)
		}
	}

	// Verify Python SDK output
	pyClient := filepath.Join(outDir, "python", "zqk_client", "client.py")
	pyInit := filepath.Join(outDir, "python", "zqk_client", "__init__.py")
	pyToml := filepath.Join(outDir, "python", "pyproject.toml")
	for _, f := range []string{pyClient, pyInit, pyToml} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("expected Python file missing: %s", f)
		}
	}

	pyBytes, err := os.ReadFile(pyClient)
	if err != nil {
		t.Fatalf("failed reading py client: %v", err)
	}
	pyContent := string(pyBytes)
	for _, sym := range []string{"class ZqkClient:", "class ZqkApiError(Exception):", "def get_health", "def get_version", "def list_objects"} {
		if !strings.Contains(pyContent, sym) {
			t.Errorf("Python SDK missing expected symbol/method: %q", sym)
		}
	}

	// Verify Go SDK output
	goClient := filepath.Join(outDir, "go", "client.go")
	goMod := filepath.Join(outDir, "go", "go.mod")
	for _, f := range []string{goClient, goMod} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("expected Go file missing: %s", f)
		}
	}

	goBytes, err := os.ReadFile(goClient)
	if err != nil {
		t.Fatalf("failed reading go client: %v", err)
	}
	goContent := string(goBytes)
	for _, sym := range []string{"package zqkclient", "type Client struct", "func NewClient(", "GetHealth", "GetVersion", "ListObjects"} {
		if !strings.Contains(goContent, sym) {
			t.Errorf("Go SDK missing expected symbol/method: %q", sym)
		}
	}
}

// TestOpenAPIClientSDK_BoundaryAndErrorHandling satisfies CRIT-1789710618301221000-8141aca2.
// Validates rejection of invalid/malformed specs, missing operation IDs, and unsupported target languages.
func TestOpenAPIClientSDK_BoundaryAndErrorHandling(t *testing.T) {
	repoRoot := findRepoRootForSDK(t)
	genScript := filepath.Join(repoRoot, "scripts", "generate_openapi_clients.py")
	tmpDir := t.TempDir()

	// Sub-test 1: Missing OpenAPI spec file
	t.Run("MissingSpecFile", func(t *testing.T) {
		cmd := exec.Command("python3", genScript, "--spec", filepath.Join(tmpDir, "non_existent.yaml"), "--out-dir", filepath.Join(tmpDir, "out1"))
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected error for missing spec file, but command succeeded:\n%s", string(out))
		}
	})

	// Sub-test 2: Malformed / non-dictionary YAML
	t.Run("MalformedSpecYAML", func(t *testing.T) {
		badYaml := filepath.Join(tmpDir, "bad.yaml")
		if err := os.WriteFile(badYaml, []byte("just a string, not a valid openapi dict\n"), 0o644); err != nil {
			t.Fatalf("failed to write bad.yaml: %v", err)
		}
		cmd := exec.Command("python3", genScript, "--spec", badYaml, "--out-dir", filepath.Join(tmpDir, "out2"))
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected error for malformed spec YAML, but command succeeded:\n%s", string(out))
		}
	})

	// Sub-test 3: Spec missing paths or openapi tag
	t.Run("SpecMissingPaths", func(t *testing.T) {
		noPathsYaml := filepath.Join(tmpDir, "nopaths.yaml")
		content := "openapi: 3.0.0\ninfo:\n  title: Empty\n  version: 1.0.0\npaths: {}\n"
		if err := os.WriteFile(noPathsYaml, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write nopaths.yaml: %v", err)
		}
		cmd := exec.Command("python3", genScript, "--spec", noPathsYaml, "--out-dir", filepath.Join(tmpDir, "out3"))
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected error for empty paths, but command succeeded:\n%s", string(out))
		}
	})

	// Sub-test 4: Path operation missing operationId
	t.Run("OperationMissingOperationId", func(t *testing.T) {
		noOpIdYaml := filepath.Join(tmpDir, "noopid.yaml")
		content := `openapi: 3.0.0
info:
  title: Test
  version: 1.0.0
paths:
  /test:
    get:
      summary: Missing operationId
      responses:
        '200':
          description: ok
`
		if err := os.WriteFile(noOpIdYaml, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write noopid.yaml: %v", err)
		}
		cmd := exec.Command("python3", genScript, "--spec", noOpIdYaml, "--out-dir", filepath.Join(tmpDir, "out4"))
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected error for missing operationId, but command succeeded:\n%s", string(out))
		}
	})

	// Sub-test 5: Unsupported target language
	t.Run("UnsupportedLanguage", func(t *testing.T) {
		specPath := filepath.Join(repoRoot, "docs", "api", "openapi.yaml")
		cmd := exec.Command("python3", genScript, "--spec", specPath, "--out-dir", filepath.Join(tmpDir, "out5"), "--languages", "ruby")
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected error for unsupported language 'ruby', but command succeeded:\n%s", string(out))
		}
		if !strings.Contains(string(out), "Unsupported target language") {
			t.Errorf("expected 'Unsupported target language' in error output, got:\n%s", string(out))
		}
	})
}

// TestOpenAPIClientSDK_IntegrationAndConformance satisfies CRIT-1789710618301222000-ed6b94df.
// Verifies verify mode, Go AST parsing conformance, and packaging compatibility.
func TestOpenAPIClientSDK_IntegrationAndConformance(t *testing.T) {
	repoRoot := findRepoRootForSDK(t)
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "sdk")

	// Generate SDK
	cmd := exec.Command("bash", filepath.Join(repoRoot, "scripts", "generate-openapi-clients.sh"), outDir)
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate-openapi-clients.sh failed: %v\nOutput:\n%s", err, string(out))
	}

	// Verify mode on valid output
	vCmd := exec.Command("bash", filepath.Join(repoRoot, "scripts", "generate-openapi-clients.sh"), "--verify", outDir)
	vCmd.Dir = repoRoot
	if out, err := vCmd.CombinedOutput(); err != nil {
		t.Fatalf("generate-openapi-clients.sh --verify failed on valid directory: %v\nOutput:\n%s", err, string(out))
	}

	// Verify mode on corrupted/missing output fails cleanly
	corruptedDir := filepath.Join(tmpDir, "corrupted")
	if err := os.MkdirAll(corruptedDir, 0o755); err != nil {
		t.Fatalf("failed to create corrupted dir: %v", err)
	}
	vFailCmd := exec.Command("bash", filepath.Join(repoRoot, "scripts", "generate-openapi-clients.sh"), "--verify", corruptedDir)
	vFailCmd.Dir = repoRoot
	if out, err := vFailCmd.CombinedOutput(); err == nil {
		t.Fatalf("expected --verify to fail on empty directory, but it succeeded:\n%s", string(out))
	}

	// Go AST syntax validation
	goFilePath := filepath.Join(outDir, "go", "client.go")
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, goFilePath, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("generated Go SDK client.go failed Go AST parsing: %v", err)
	}
	if node.Name.Name != "zqkclient" {
		t.Errorf("expected package name 'zqkclient', got %q", node.Name.Name)
	}
}
