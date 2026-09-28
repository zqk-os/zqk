package community

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	packCompDocRelPath     = "docs/architecture/PACK_COMPOSITION_AND_EXTENSIBILITY.md"
	packCompStubRelPath    = "PACK-COMPOSITION.md"
	docsIndexRelPath       = "docs/INDEX.md"
	zparqlManualRelPath    = "docs/manual/ZPARQL_QUERY_LANGUAGE.md"
	zqlManualRelPath       = "docs/manual/ZQL_MUTATIONS.md"
	lifecycleDocRelPath    = "docs/architecture/LIFECYCLE_STATE_MACHINE.md"
	tokenPackManifest      = "pack.yaml"
	tokenBuilderCodegen    = "bldr_cli_cmd_v1"
	tokenMermaidFlowchart  = "mermaid"
	tokenZparqlSelect      = "SELECT"
	tokenZqlApply          = "apply"
	minDocByteLength       = 200
)

// TestPackCompositionDocs verifies the existence and completeness of modular pack composition docs.
func TestPackCompositionDocs(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	docPath := filepath.Join(root, packCompDocRelPath)
	if !fileutil.Exists(docPath) {
		t.Fatalf("expected pack composition doc at %s", docPath)
	}

	contentBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("failed to read pack composition doc: %v", err)
	}
	content := string(contentBytes)
	if len(content) < minDocByteLength {
		t.Errorf("pack composition doc content is too short (%d bytes)", len(content))
	}
	if !strings.Contains(content, tokenPackManifest) {
		t.Errorf("expected '%s' in pack composition doc", tokenPackManifest)
	}
	if !strings.Contains(content, tokenBuilderCodegen) {
		t.Errorf("expected '%s' in pack composition doc", tokenBuilderCodegen)
	}

	// Verify linkage in PACK-COMPOSITION.md
	stubPath := filepath.Join(root, packCompStubRelPath)
	if fileutil.Exists(stubPath) {
		stubBytes, readErr := os.ReadFile(stubPath)
		if readErr == nil && !strings.Contains(string(stubBytes), "PACK_COMPOSITION_AND_EXTENSIBILITY.md") {
			t.Errorf("expected link to PACK_COMPOSITION_AND_EXTENSIBILITY.md in %s", packCompStubRelPath)
		}
	}
}

// TestLanguageDocs verifies the existence and completeness of ZPARQL and ZQL manuals.
func TestLanguageDocs(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	zparqlPath := filepath.Join(root, zparqlManualRelPath)
	if !fileutil.Exists(zparqlPath) {
		t.Fatalf("expected ZPARQL manual at %s", zparqlPath)
	}
	zparqlBytes, err := os.ReadFile(zparqlPath)
	if err != nil {
		t.Fatalf("failed to read ZPARQL manual: %v", err)
	}
	zparqlContent := string(zparqlBytes)
	if len(zparqlContent) < minDocByteLength || !strings.Contains(zparqlContent, tokenZparqlSelect) {
		t.Errorf("ZPARQL manual incomplete or missing SELECT token")
	}

	zqlPath := filepath.Join(root, zqlManualRelPath)
	if !fileutil.Exists(zqlPath) {
		t.Fatalf("expected ZQL manual at %s", zqlPath)
	}
	zqlBytes, zqlErr := os.ReadFile(zqlPath)
	if zqlErr != nil {
		t.Fatalf("failed to read ZQL manual: %v", zqlErr)
	}
	zqlContent := string(zqlBytes)
	if len(zqlContent) < minDocByteLength || !strings.Contains(zqlContent, tokenZqlApply) {
		t.Errorf("ZQL manual incomplete or missing apply token")
	}
}

// TestLifecycleDocs verifies the visual lifecycle state machine documentation.
func TestLifecycleDocs(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	docPath := filepath.Join(root, lifecycleDocRelPath)
	if !fileutil.Exists(docPath) {
		t.Fatalf("expected lifecycle state machine doc at %s", docPath)
	}

	contentBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("failed to read lifecycle state machine doc: %v", err)
	}
	content := string(contentBytes)
	if len(content) < minDocByteLength {
		t.Errorf("lifecycle state machine doc content is too short (%d bytes)", len(content))
	}
	if !strings.Contains(content, tokenMermaidFlowchart) {
		t.Errorf("expected mermaid diagrams in lifecycle doc")
	}
}
