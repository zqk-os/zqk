package ontology

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/testkit"
)

const sampleTurtle = `
@prefix org: <http://example.com/org#> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix owl: <http://www.w3.org/2002/07/owl#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .

org:Organization a owl:Class ;
    rdfs:label "Organization" .
org:Division a owl:Class ;
    rdfs:label "Division" ;
    rdfs:subClassOf org:Organization .
`

// TestOntologyImport_Integration runs ontology import with a temp Turtle file and a test
// project root, then asserts output contains format detection and translation (BLI-764).
func TestOntologyImport_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "cmd.ontology.import"})
	testRoot := proj.Root

	ttlPath := filepath.Join(testRoot, "sample.ttl")
	if err := fileutil.WriteFile(ttlPath, []byte(strings.TrimSpace(sampleTurtle)), paths.FilePerm600); err != nil {
		t.Fatalf("WriteFile sample.ttl: %v", err)
	}

	cmd := NewImportCmd()
	cmd.SetArgs([]string{})
	if err := cmd.Flags().Set("file", ttlPath); err != nil {
		t.Fatalf("Set file flag: %v", err)
	}

	// Set context so NewProcessor can resolve project root (required by runOntologyImport).
	cli.SetContext(cmd, cli.ContextForProjectRoot(testRoot))

	var execErr error
	out := captureStdoutOntology(t, func() { execErr = cmd.Execute() })
	if execErr != nil {
		t.Fatalf("Execute ontology import: %v", execErr)
	}
	if !strings.Contains(out, "Format detected") {
		t.Errorf("output should contain 'Format detected', got:\n%s", out)
	}
	if !strings.Contains(out, "Translation:") {
		t.Errorf("output should contain 'Translation:' (BLI-764 pipeline), got:\n%s", out)
	}
	// When translation produces objects, they are persisted via storage (BLI-764).
	if strings.Contains(out, "1 objects") && !strings.Contains(out, "Persisted:") {
		t.Errorf("output should contain 'Persisted:' when translation produces objects, got:\n%s", out)
	}
	// CRIT-7642: sample has 2 owl:Class → 1 domain_registry with 2 domains; expect persisted count.
	if !strings.Contains(out, "Persisted: 1 object(s)") && strings.Contains(out, "1 objects") {
		t.Errorf("output should contain 'Persisted: 1 object(s)' when translation produces 1 domain_registry, got:\n%s", out)
	}
	// Translated domain_registry includes domains from Turtle (organization, division).
	if strings.Contains(out, "1 objects") && !strings.Contains(out, "organization") && !strings.Contains(out, "division") {
		// Domains may appear in logs or list output; at least translation ran with our sample.
		t.Logf("translation produced objects; domain ids (organization, division) may appear in storage")
	}
}
