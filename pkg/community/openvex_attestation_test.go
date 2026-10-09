package community

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// OpenVEXDocument models the CISA OpenVEX v0.2.0 schema for test validation (BLI-1789709883360168000-82bffef0)
type OpenVEXDocument struct {
	Context    string             `json:"@context"`
	ID         string             `json:"@id"`
	Author     string             `json:"author"`
	Role       string             `json:"role"`
	Timestamp  string             `json:"timestamp"`
	Version    int                `json:"version"`
	Tooling    string             `json:"tooling"`
	Statements []OpenVEXStatement `json:"statements"`
}

type OpenVEXStatement struct {
	Vulnerability   OpenVEXVuln   `json:"vulnerability"`
	Products        []OpenVEXProd `json:"products"`
	Status          string        `json:"status"`
	Justification   string        `json:"justification,omitempty"`
	ImpactStatement string        `json:"impact_statement,omitempty"`
	ActionStatement string        `json:"action_statement,omitempty"`
}

type OpenVEXVuln struct {
	Name string `json:"name"`
}

type OpenVEXProd struct {
	ID string `json:"@id"`
}

// TestOpenVEX_FunctionalAcceptance verifies that generate-openvex.sh generates
// a valid CISA OpenVEX v0.2.0 document with required statements and attestation metadata
// (CRIT-1789709849855043000-bcecc9a7).
func TestOpenVEX_FunctionalAcceptance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	scriptPath := filepath.Join(root, "scripts", "generate-openvex.sh")
	if !fileutil.Exists(scriptPath) {
		t.Fatalf("expected generate-openvex.sh script at %s", scriptPath)
	}

	tmpDir := t.TempDir()
	outVex := filepath.Join(tmpDir, "openvex.json")

	cmd := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, "v2.8.0", outVex)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generate-openvex.sh failed: %v, output: %s", err, string(out))
	}

	if !fileutil.Exists(outVex) {
		t.Fatalf("expected output file at %s", outVex)
	}

	vexBytes, err := fileutil.ReadFile(outVex)
	if err != nil {
		t.Fatalf("failed to read generated OpenVEX file: %v", err)
	}

	var doc OpenVEXDocument
	if err := json.Unmarshal(vexBytes, &doc); err != nil {
		t.Fatalf("failed to parse OpenVEX JSON: %v", err)
	}

	if !strings.HasPrefix(doc.Context, "https://openvex.dev/ns") {
		t.Errorf("expected @context to start with 'https://openvex.dev/ns', got '%s'", doc.Context)
	}
	if doc.ID == "" {
		t.Errorf("expected non-empty @id in OpenVEX document")
	}
	if !strings.Contains(doc.Author, "Zen Quantum Kernel Security Response Team") {
		t.Errorf("expected author to contain 'Zen Quantum Kernel Security Response Team'")
	}
	if len(doc.Statements) == 0 {
		t.Fatalf("expected at least one statement in OpenVEX document")
	}

	for i, stmt := range doc.Statements {
		if stmt.Vulnerability.Name == "" {
			t.Errorf("statement %d missing vulnerability.name", i)
		}
		if len(stmt.Products) == 0 {
			t.Errorf("statement %d missing products", i)
		}
		if stmt.Status == "" {
			t.Errorf("statement %d missing status", i)
		}
		if stmt.Status == "not_affected" && stmt.Justification == "" && stmt.ImpactStatement == "" {
			t.Errorf("statement %d with not_affected requires justification or impact_statement", i)
		}
	}
}

// TestOpenVEX_BoundaryAndErrorHandling verifies verification mode and error handling
// on nonexistent files, malformed JSON, and invalid OpenVEX statements
// (CRIT-1789709849855044000-44a4eb5e).
func TestOpenVEX_BoundaryAndErrorHandling(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	scriptPath := filepath.Join(root, "scripts", "generate-openvex.sh")

	// 1. Missing verify argument
	cmdNoArg := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, "--verify")
	if err := cmdNoArg.Run(); err == nil {
		t.Errorf("expected --verify without file argument to fail")
	}

	// 2. Nonexistent file
	cmdNonexistent := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, "--verify", "/nonexistent/openvex.json")
	if err := cmdNonexistent.Run(); err == nil {
		t.Errorf("expected --verify on nonexistent file to fail")
	}

	// 3. Corrupt JSON
	tmpDir := t.TempDir()
	badJSON := filepath.Join(tmpDir, "bad.json")
	if err := fileutil.WriteFile(badJSON, []byte("{invalid json content"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write bad json: %v", err)
	}
	cmdBad := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, "--verify", badJSON)
	if err := cmdBad.Run(); err == nil {
		t.Errorf("expected --verify on corrupt JSON to fail")
	}

	// 4. Invalid OpenVEX schema (missing statements)
	invalidVex := filepath.Join(tmpDir, "invalid-vex.json")
	invalidContent := `{"@context":"https://openvex.dev/ns/v0.2.0","@id":"test","author":"test","statements":[]}`
	if err := fileutil.WriteFile(invalidVex, []byte(invalidContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write invalid vex: %v", err)
	}
	cmdInvalid := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, "--verify", invalidVex)
	if err := cmdInvalid.Run(); err == nil {
		t.Errorf("expected --verify on document with empty statements to fail")
	}
}

// TestOpenVEX_IntegrationAndConformance verifies release packaging
// and manifest checksum integration for OpenVEX security attestations
// (CRIT-1789709849855045000-c3474b7c).
func TestOpenVEX_IntegrationAndConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	packageScript := filepath.Join(root, "scripts", "package-community.sh")
	if !fileutil.Exists(packageScript) {
		t.Fatalf("expected package-community.sh at %s", packageScript)
	}

	packageBytes, err := fileutil.ReadFile(packageScript)
	if err != nil {
		t.Fatalf("failed to read package-community.sh: %v", err)
	}
	packageContent := string(packageBytes)

	if !strings.Contains(packageContent, "generate-openvex.sh") {
		t.Errorf("expected package-community.sh to invoke generate-openvex.sh")
	}
	if !strings.Contains(packageContent, "openvex.json") {
		t.Errorf("expected package-community.sh to reference openvex.json")
	}

	// Test end-to-end generation and verification
	tmpDir := t.TempDir()
	genScript := filepath.Join(root, "scripts", "generate-openvex.sh")
	vexPath := filepath.Join(tmpDir, "openvex.json")

	if out, err := testkit.ManagedCommand(t, t.Context(), "bash", genScript, "v3.0.0", vexPath).CombinedOutput(); err != nil {
		t.Fatalf("failed to generate OpenVEX doc: %v, out: %s", err, string(out))
	}

	verifyCmd := testkit.ManagedCommand(t, t.Context(), "bash", genScript, "--verify", vexPath)
	out, err := verifyCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected --verify on generated OpenVEX doc to pass: %v, out: %s", err, string(out))
	}
	if !strings.Contains(string(out), "conforms to OpenVEX") {
		t.Errorf("expected confirmation message in verify output, got: %s", string(out))
	}
}
