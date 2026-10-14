package ontology

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/translation"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupOntologyIsolatedProject(t *testing.T) string {
	t.Helper()
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cmd.ontology.unit",
	})
	return proj.Root
}

func TestNewOntologyCmd(t *testing.T) {
	cmd := NewOntologyCmd()
	if cmd == nil {
		t.Fatal("expected NewOntologyCmd to return non-nil command")
	}
	if cmd.Use != "ontology" {
		t.Errorf("expected command Use 'ontology', got %q", cmd.Use)
	}

	subCommands := cmd.Commands()
	var hasImport bool
	for _, sub := range subCommands {
		if sub.Name() == "import" {
			hasImport = true
		}
	}
	if !hasImport {
		t.Error("expected 'import' subcommand to be present under ontology")
	}

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute ontology --help: %v", err)
	}
	if !strings.Contains(buf.String(), "Ontology operations for importing") {
		t.Errorf("expected help output to describe ontology operations, got:\n%s", buf.String())
	}
}

func TestDetectImportFormat_ExplicitFlags(t *testing.T) {
	tests := []struct {
		formatFlag    string
		expectedKey   string
		expectedLabel string
	}{
		{"rdf_owl", "rdf_owl", "RDF/OWL"},
		{"turtle", "turtle", "Turtle"},
		{"rdf_xml", "rdf_xml", "RDF/XML"},
		{"jsonld", "jsonld", "JSON-LD"},
		{"cypher", "cypher", "Cypher"},
		{"json_schema", "json_schema", "JSON Schema"},
		{"openapi", "openapi", "OpenAPI"},
		{"custom_unknown", "custom_unknown", "custom_unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.formatFlag, func(t *testing.T) {
			k, l := detectImportFormat("sample.txt", "", tc.formatFlag)
			if k != tc.expectedKey || l != tc.expectedLabel {
				t.Errorf("detectImportFormat with flag %q: got (%q, %q), want (%q, %q)",
					tc.formatFlag, k, l, tc.expectedKey, tc.expectedLabel)
			}
		})
	}
}

func TestDetectImportFormat_ContentAndExtensionDetection(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		content       string
		expectedKey   string
		expectedLabel string
	}{
		{"CypherExtension", "graph.cypher", "MATCH (n) RETURN n", "cypher", "Cypher"},
		{"CypherConstraint", "query.txt", "CREATE CONSTRAINT ON (n:Person) ASSERT n.id IS UNIQUE", "cypher", "Cypher"},
		{"CypherIndex", "query.txt", "CREATE INDEX ON (n:User)", "cypher", "Cypher"},
		{"OpenAPIPrefix", "api.yaml", "openapi: 3.0.0\ninfo:\n  title: Test", "openapi", "OpenAPI"},
		{"SwaggerPrefix", "api.yaml", "swagger: 2.0\ninfo:\n  title: Test", "openapi", "OpenAPI"},
		{"OpenAPINewline", "api.yaml", "# Comment\nopenapi: 3.0.1", "openapi", "OpenAPI"},
		{"SwaggerNewline", "api.yaml", "# Comment\nswagger: 2.0", "openapi", "OpenAPI"},
		{"JSONSchema", "schema.json", "{\"$schema\": \"http://json-schema.org/draft-07/schema#\", \"definitions\": {}}", "json_schema", "JSON Schema"},
		{"JSONSchemaRaw", "blob.txt", "{\n  \"$schema\": \"v1\",\n  \"definitions\": {}\n}", "json_schema", "JSON Schema"},
		{"TurtleExt", "model.ttl", "", "turtle", "Turtle"},
		{"OWLExt", "ontology.owl", "", "rdf_owl", "RDF/OWL"},
		{"RDFExt", "data.rdf", "", "rdf_xml", "RDF/XML"},
		{"XMLExt", "data.xml", "", "rdf_xml", "RDF/XML"},
		{"JSONLDExt", "context.jsonld", "{}", "jsonld", "JSON-LD"},
		{"JSONWithContext", "context.json", "{\"@context\": {\"name\": \"http://schema.org/name\"}}", "jsonld", "JSON-LD"},
		{"JSONWithID", "entity.json", "{\"@id\": \"urn:test:1\"}", "jsonld", "JSON-LD"},
		{"PrefixInContent", "untyped.txt", "@prefix ex: <http://example.org/> .", "turtle", "Turtle"},
		{"PredicateAInContent", "untyped.txt", ":s a :Class .", "turtle", "Turtle"},
		{"FallbackDefault", "untyped.txt", "unknown text content", "rdf_owl", "RDF/OWL"},
		{"AutoFlagMapping", "model.ttl", "", "turtle", "Turtle"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			formatFlag := ""
			if tc.name == "AutoFlagMapping" {
				formatFlag = "auto"
			}
			k, l := detectImportFormat(tc.path, tc.content, formatFlag)
			if k != tc.expectedKey || l != tc.expectedLabel {
				t.Errorf("detectImportFormat(%q, %q): got (%q, %q), want (%q, %q)",
					tc.path, tc.content, k, l, tc.expectedKey, tc.expectedLabel)
			}
		})
	}
}

func TestOntologyImport_MissingAndInvalidFile(t *testing.T) {
	testRoot := setupOntologyIsolatedProject(t)

	t.Run("MissingFileFlag", func(t *testing.T) {
		cmd := NewImportCmd()
		cmd.SetArgs([]string{})
		cli.SetContext(cmd, cli.ContextForProjectRoot(testRoot))
		var err error
		captureStdoutOntology(t, func() { err = cmd.Execute() })
		if err == nil || !strings.Contains(err.Error(), "--file is required") {
			t.Fatalf("expected '--file is required' error, got %v", err)
		}
	})

	t.Run("NonExistentFilePath", func(t *testing.T) {
		cmd := NewImportCmd()
		cmd.SetArgs([]string{})
		_ = cmd.Flags().Set("file", filepath.Join(testRoot, "non_existent_file_path_12345.ttl"))
		cli.SetContext(cmd, cli.ContextForProjectRoot(testRoot))
		var err error
		captureStdoutOntology(t, func() { err = cmd.Execute() })
		if err == nil || !strings.Contains(err.Error(), "failed to read file") {
			t.Fatalf("expected 'failed to read file' error, got %v", err)
		}
	})

	t.Run("EmptyFile", func(t *testing.T) {
		emptyPath := filepath.Join(testRoot, "empty_ontology.ttl")
		if err := fileutil.WriteFile(emptyPath, []byte(""), paths.FilePerm600); err != nil {
			t.Fatalf("write empty file failed: %v", err)
		}
		cmd := NewImportCmd()
		cmd.SetArgs([]string{})
		_ = cmd.Flags().Set("file", emptyPath)
		cli.SetContext(cmd, cli.ContextForProjectRoot(testRoot))
		var err error
		out := captureStdoutOntology(t, func() { err = cmd.Execute() })
		if err != nil {
			t.Fatalf("empty file import execution failed: %v", err)
		}
		if !strings.Contains(out, "0 bytes, 0 lines") {
			t.Errorf("expected output to report '0 bytes, 0 lines', got:\n%s", out)
		}
	})
}

func TestOntologyImport_PersistTranslatedObjectsBranches(t *testing.T) {
	testRoot := setupOntologyIsolatedProject(t)

	cmd := NewImportCmd()
	cmd.SetContext(pkgctx.NewSystemContext())
	cli.SetContext(cmd, cli.ContextForProjectRoot(testRoot))
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		t.Fatalf("NewProcessor failed: %v", err)
	}
	ctx, secCtx, store := proc.StorageTuple()

	// 1. TranslatedObject with nil Data
	itemsNil := []translation.TranslatedObject{
		{Kind: "domain_registry", Data: nil},
	}
	persisted0 := persistTranslatedObjects(ctx, secCtx, store, itemsNil, proc)
	if persisted0 != 0 {
		t.Errorf("expected 0 persisted for nil Data, got %d", persisted0)
	}

	// 2. TranslatedObject with empty id and non-placeholder kind
	itemsNoID := []translation.TranslatedObject{
		{Kind: "custom_kind", Data: map[string]any{"title": "No ID"}},
	}
	persistedNoID := persistTranslatedObjects(ctx, secCtx, store, itemsNoID, proc)
	if persistedNoID != 0 {
		t.Errorf("expected 0 persisted for item without ID, got %d", persistedNoID)
	}

	// 3. TranslatedObject with DOMAIN-REG-000 placeholder ID
	itemsPlaceholder := []translation.TranslatedObject{
		{
			Kind: "domain_registry",
			Data: map[string]any{
				"id":    "DOMAIN-REG-000",
				"title": "Generated Domain Registry",
			},
		},
	}
	persistedPH := persistTranslatedObjects(ctx, secCtx, store, itemsPlaceholder, proc)
	if persistedPH != 1 {
		t.Errorf("expected 1 persisted for placeholder ID domain_registry, got %d", persistedPH)
	}
}
