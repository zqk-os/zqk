package explain

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/acronyms"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/testkit"
)

// TestExplainCmd_Execution proves CRIT-1790814939731525000-2151219e (Functional Acceptance).
func TestExplainCmd_Execution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})

	// Sync canonical terms into the test project kernel storage
	secCtx := pkgctx.NewSystemSecurityContext()
	synced, err := acronyms.SyncToKernel(t.Context(), secCtx, proj.FileStorage)
	if err != nil {
		t.Fatalf("failed to sync acronyms to kernel: %v", err)
	}
	if synced == 0 {
		t.Fatal("expected at least one term synced")
	}

	cmd := NewExplainCmd()
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetArgs([]string{"BLI"})

	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("unexpected error executing explain BLI: %v", err)
	}
	if !strings.Contains(buf.String(), "Backlog Item") {
		t.Fatalf("expected output to contain 'Backlog Item', got: %s", buf.String())
	}

	// Verify required output fields when listing all terms
	cmdList := NewExplainCmd()
	var listBuf bytes.Buffer
	listCtx := pkgctx.WithCommandOutputWriter(context.Background(), &listBuf)
	cmdList.SetArgs([]string{})
	if err := cmdList.ExecuteContext(listCtx); err != nil {
		t.Fatalf("unexpected error executing explain list: %v", err)
	}
	if !strings.Contains(listBuf.String(), "BLI") || !strings.Contains(listBuf.String(), "PRI") {
		t.Fatalf("expected list output to contain BLI and PRI, got: %s", listBuf.String())
	}
}

// TestExplainCmd_DynamicGlossaryTermDiscovery proves that any glossary_term created in the kernel
// is immediately discoverable by zqk explain without hardcoding or code changes.
func TestExplainCmd_DynamicGlossaryTermDiscovery(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	secCtx := pkgctx.NewSystemSecurityContext()

	// Ensure the parent vocabulary_scheme exists
	schemeObj := map[string]any{
		"id":             acronyms.KernelAcronymsSchemeID,
		"kind":           "vocabulary_scheme",
		"title":          "Kernel Acronym & Jargon Taxonomy",
		"status":         "active",
		"context_scope":  "operational",
		"purpose":        "mixed",
		"summary":        "Core system acronyms.",
		"namespace_id":   "zqk:kernel",
		"schema_version": "2.0.0",
		"source_type":    "internal",
	}
	if err := proj.FileStorage.Create(t.Context(), secCtx, schemeObj); err != nil {
		t.Fatalf("failed to create vocabulary_scheme: %v", err)
	}

	// Create a new domain-specific glossary term under VOC-KERNEL-ACRONYMS
	customTerm := acronyms.Acronym{
		Code:        "MYNEWACR",
		FullName:    "My New Custom Acronym",
		SchemeRef:   acronyms.KernelAcronymsSchemeID,
		Category:    "custom",
		Definition:  "A dynamic acronym created in kernel storage as a glossary_term object.",
		Context:     "operational",
		RelatedRefs: []string{"BLI"},
	}

	objMap := customTerm.ToGlossaryTerm()
	if err := proj.FileStorage.Create(t.Context(), secCtx, objMap); err != nil {
		t.Fatalf("failed to create glossary_term in storage: %v", err)
	}

	cmd := NewExplainCmd()
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetArgs([]string{"MYNEWACR"})

	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("failed to explain newly created glossary_term: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "MYNEWACR") || !strings.Contains(output, "My New Custom Acronym") {
		t.Fatalf("expected dynamic discovery of MYNEWACR, got: %s", output)
	}
}

// TestExplainCmd_NegativeBoundary proves CRIT-1790814939731526000-641cda96 (Boundary & Error Handling).
func TestExplainCmd_NegativeBoundary(t *testing.T) {
	_ = testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})

	cmd := NewExplainCmd()
	cmd.SetArgs([]string{"NONEXISTENT"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent acronym, got nil")
	}
	if !strings.Contains(err.Error(), "unknown kernel acronym") {
		t.Errorf("expected error message to mention unknown kernel acronym, got: %v", err)
	}

	// Verify fuzzy suggestion
	cmdFuzzy := NewExplainCmd()
	cmdFuzzy.SetArgs([]string{"BL"})
	errFuzzy := cmdFuzzy.Execute()
	if errFuzzy == nil {
		t.Fatal("expected error for BL, got nil")
	}
	if !strings.Contains(errFuzzy.Error(), "Did you mean: BLI?") {
		t.Errorf("expected fuzzy suggestion for BL, got: %v", errFuzzy)
	}
}
